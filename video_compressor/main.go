package main

import (
	"io/ioutil"
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
	completed := make(chan string, len(videoPaths))

	go func(completedChan chan string) {
		for input := range completedChan {
			wg.Add(1)
			semaphoreDel <- struct{}{} // Acquire the semaphore
			isNeeded := isVideoStillNeeded(input, counter, videoPaths)
			if isNeeded {
				log.Printf("Skipping removal of %s\n", input)
				<-semaphoreDel // Release the semaphore
				wg.Done()
				continue
			}

			log.Printf("Finished compressing %s\nDo you want to remove the original file? (Y/N): \n", input)
			// reader := bufio.NewReader(os.Stdin)
			// answer, _ := reader.ReadString('\n')
			// answer = strings.ToUpper(strings.TrimSpace(answer))
			answer := "Y"

			if answer == "Y" {
				err := os.Remove(input)
				if err != nil {
					log.Printf("Error removing original file %s: %v", input, err)
				} else {
					log.Printf("Removed original file: %s\n", input)
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
			defer wg.Done()
			err := compressVideo(e.VideoPath, e.OutputPath, e.StartTime, e.EndTime)
			counter++
			log.Printf("Progress: %d/%d\n", counter, len(videoPaths))
			<-semaphore // Release the semaphore
			if err != nil {
				log.Printf("Error compressing video: %v", err)
				// return
			}
			completed <- e.VideoPath
		}(path)
	}

	go func() {
		wg.Wait()
		close(completed)
	}()
}

type InputEntry struct {
	VideoPath  string
	OutputPath string
	StartTime  *string
	EndTime    *string
}

func isVideoStillNeeded(videoPath string, counter int, videoPaths []InputEntry) bool {
	for i := counter; i < len(videoPaths); i++ {
		if videoPaths[i].VideoPath == videoPath {
			return true
		}
	}
	return false
}

func readConfig(filename string) ([]InputEntry, error) {
	content, err := ioutil.ReadFile(filename)
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
		"crf":    "23",
		"preset": "superfast",
		"c:a":    "aac",
		"b:a":    "128k",
		"tag:v":  "hvc1",
	}

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
