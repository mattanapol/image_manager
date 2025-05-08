package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mattanapol/image_manager/internal/file_helper"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

func main() {
	videoPaths, err := readConfig("input.txt")
	if err != nil {
		log.Fatalf("Error reading input file: %v", err)
	}

	var wg sync.WaitGroup
	var counter int = 0
	semaphore := make(chan struct{}, 1)
	semaphoreDel := make(chan struct{}, 1)
	completed := make(chan CompletedEntry, len(videoPaths))

	go func(completedChan chan CompletedEntry) {
		for input := range completedChan {
			semaphoreDel <- struct{}{} // Acquire the semaphore
			isNeeded := isVideoStillNeeded(input.InputVideoPath, counter, videoPaths)
			isSmaller, err := isNewFileSmaller(input.InputVideoPath, input.OutputVideoPath)
			if err != nil {
				log.Printf("Error comparing file sizes: %v", err)
			}
			if isNeeded {
				log.Printf("Skipping removal of %s\n", input)
				<-semaphoreDel // Release the semaphore
				wg.Done()
				continue
			}

			// log.Printf("Finished compressing %s\nDo you want to remove the original file? (Y/N): \n", input)
			// reader := bufio.NewReader(os.Stdin)
			// answer, _ := reader.ReadString('\n')
			// answer = strings.ToUpper(strings.TrimSpace(answer))
			answer := "Y"

			if answer == "Y" && isSmaller {
				err := os.Remove(input.InputVideoPath)
				if err != nil {
					log.Printf("Error removing original file %s: %v", input, err)
				} else {
					log.Printf("Removed original file: %s\n", input)
				}
			}
			if !isSmaller {
				log.Println("The new file is larger than the original file. Remove new file")
				err := os.Remove(input.OutputVideoPath)
				if err != nil {
					log.Printf("Error removing new file %s: %v", input, err)
				} else {
					log.Printf("Removed new file: %s\n", input.OutputVideoPath)
				}
			}
			<-semaphoreDel // Release the semaphore
			wg.Done()
		}
	}(completed)

	for _, path := range videoPaths {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire the semaphore
		go func(e InputEntry) {
			err := compressVideo(e.VideoPath, e.OutputPath, e.StartTime, e.EndTime)
			counter++
			log.Printf("Progress: %d/%d\n", counter, len(videoPaths))
			<-semaphore // Release the semaphore
			if err != nil {
				log.Printf("Error compressing video: %v", err)
				// return
			}
			completed <- CompletedEntry{
				InputVideoPath:  e.VideoPath,
				OutputVideoPath: e.OutputPath,
			}
		}(path)
	}

	wg.Wait()
	close(completed)
}

type InputEntry struct {
	VideoPath  string
	OutputPath string
	StartTime  *string
	EndTime    *string
}

type CompletedEntry struct {
	InputVideoPath  string
	OutputVideoPath string
}

func isVideoStillNeeded(videoPath string, counter int, videoPaths []InputEntry) bool {
	for i := counter; i < len(videoPaths); i++ {
		if videoPaths[i].VideoPath == videoPath {
			return true
		}
	}
	return false
}

// Compare size of two files
func isNewFileSmaller(oldFilePath, newFilePath string) (bool, error) {
	oldFileInfo, err := os.Stat(oldFilePath)
	if err != nil {
		return false, err
	}

	newFileInfo, err := os.Stat(newFilePath)
	if err != nil {
		return false, err
	}
	if oldFileInfo.IsDir() || newFileInfo.IsDir() {
		return false, nil
	}

	log.Printf("Old file size: %d, New file size: %d\n", oldFileInfo.Size()/(1024*1024), newFileInfo.Size()/(1024*1024))

	return newFileInfo.Size() <= oldFileInfo.Size(), nil
}

func readConfig(filename string) ([]InputEntry, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	var paths []InputEntry
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			// get comma separated values
			values := strings.Split(line, ",")
			if len(values) == 1 {
				input := values[0]
				paths = append(paths, InputEntry{
					VideoPath:  input,
					OutputPath: createOutputPath(input),
				})
				continue
			} else if len(values) == 2 {
				input := values[0]
				paths = append(paths, InputEntry{
					VideoPath:  input,
					OutputPath: createOutputPath(input),
					StartTime:  &values[1],
				})
				continue
			} else if len(values) == 3 {
				input := values[0]
				paths = append(paths, InputEntry{
					VideoPath:  input,
					OutputPath: createOutputPath(input),
					StartTime:  &values[1],
					EndTime:    &values[2],
				})
				continue
			} else if len(values) == 4 {
				paths = append(paths, InputEntry{
					VideoPath:  values[0],
					OutputPath: values[3],
					StartTime:  &values[1],
					EndTime:    &values[2],
				})
				continue
			} else {
				log.Printf("Invalid input: %s\n", line)
				continue
			}
		}
	}
	return paths, nil
}

func createOutputPath(input string) string {
	ext := filepath.Ext(input)
	output := strings.TrimSuffix(input, ext) + "_compressed.mp4"
	return output
}

func compressVideo(input, output string,
	startTime, endTime *string) error {
	// a, err := ffmpeg.Probe(input)
	// if err != nil {
	// 	log.Println("Error reading input file:", input)
	// 	panic(err)
	// }
	// totalDuration := gjson.Get(a, "format.duration").Float()

	// var args ffmpeg.KwArgs = map[string]interface{}{
	// 	"c:v":    "libx264",
	// 	"crf":    "22",
	// 	"preset": "slower",
	// 	"c:a":    "aac",
	// 	"b:a":    "128k",
	// }

	var args ffmpeg.KwArgs = map[string]interface{}{
		"c:v":    "libx265",
		"crf":    "22",
		"preset": "medium",
		"c:a":    "aac",
		"b:a":    "128k",
		"tag:v":  "hvc1",
	}

	// var args ffmpeg.KwArgs = map[string]interface{}{
	// 	"c:v":    "libx265",
	// 	"crf":    "22",
	// 	"preset": "veryfast",
	// 	"c:a":    "aac",
	// 	"b:a":    "128k",
	// 	"tag:v":  "hvc1",
	// }

	// var args ffmpeg.KwArgs = map[string]interface{}{
	// 	"c:v":    "h264_videotoolbox",
	// 	"preset": "slower",
	// 	"c:a":    "aac",
	// 	"b:a":    "128k",
	// 	"q:v":    "55",
	// }

	if startTime != nil && *startTime != "" {
		args["ss"] = *startTime
	}

	if endTime != nil && *endTime != "" {
		args["to"] = *endTime
	}

	dir := filepath.Dir(output)
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return err
	}

	nextAvailableFilePath, err := file_helper.GetNextAvailableFilePath(output)
	if err != nil {
		return err
	}
	log.Printf("Compressing %s to %s\n", input, nextAvailableFilePath)

	return ffmpeg.Input(input).
		Output(nextAvailableFilePath, args).
		// GlobalArgs("-progress", "unix://"+examples.TempSock(totalDuration)).
		// OverWriteOutput().
		Run()
}
