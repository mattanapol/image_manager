package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

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
			fmt.Printf("Finished compressing %s\nDo you want to remove the original file? (Y/N): \n", input)
			// reader := bufio.NewReader(os.Stdin)
			// answer, _ := reader.ReadString('\n')
			// answer = strings.ToUpper(strings.TrimSpace(answer))
			answer := "Y"

			if answer == "Y" {
				err := os.Remove(input)
				if err != nil {
					log.Printf("Error removing original file %s: %v", input, err)
				} else {
					fmt.Printf("Removed original file: %s\n", input)
				}
			}
			<-semaphoreDel // Release the semaphore
			wg.Done()
		}
	}(completed)

	for _, path := range videoPaths {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire the semaphore
		go func(input string) {
			defer wg.Done()
			ext := filepath.Ext(input)
			output := strings.TrimSuffix(input, ext) + "_compressed.mp4"
			err := compressVideo(input, output)
			counter++
			fmt.Printf("Progress: %d/%d\n", counter, len(videoPaths))
			<-semaphore // Release the semaphore
			if err != nil {
				log.Printf("Error compressing video: %v", err)
				// return
			}
			completed <- input
		}(path)
	}

	go func() {
		wg.Wait()
		close(completed)
	}()
}

func readConfig(filename string) ([]string, error) {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	var paths []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			paths = append(paths, line)
		}
	}
	return paths, nil
}

func compressVideo(input, output string) error {
	// a, err := ffmpeg.Probe(input)
	// if err != nil {
	// 	fmt.Println("Error reading input file:", input)
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

	return ffmpeg.Input(input).
		Output(output, args).
		// GlobalArgs("-progress", "unix://"+examples.TempSock(totalDuration)).
		// OverWriteOutput().
		Run()
}
