package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/corona10/goimagehash"
	"github.com/disintegration/imaging"
	"github.com/mattanapol/image_manager/internal/csv_helper"
)

const (
	similarityThreshold = 96
	gcInterval          = 100
)

var (
	rootFolder      = "/Volumes/SSD02"
	numberOfThreads = 2
	outputFile      = "./results.csv"
	blacklist       = []string{"$RECYCLE.BIN"}
)

type FileInfo struct {
	Path string
	Hash *goimagehash.ImageHash
}

func main() {
	start := time.Now()

	fileInfos := make(chan FileInfo)
	wg := &sync.WaitGroup{}

	wg.Add(1)
	go func() {
		walkDirectory(rootFolder, fileInfos, wg)
		wg.Wait()
		close(fileInfos)
	}()

	compareFiles(fileInfos)

	elapsed := time.Since(start)
	fmt.Printf("Elapsed time: %s\n", elapsed)
}

func walkDirectory(root string, fileInfos chan<- FileInfo, wg *sync.WaitGroup) {
	defer wg.Done()

	semaphore := make(chan struct{}, numberOfThreads)
	fileCounter := 0

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			fmt.Printf("Error accessing path %q: %v\n", path, err)
			return err
		}

		if isBlacklisted(path) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			if path != root {
				wg.Add(1)
				go walkDirectory(path, fileInfos, wg)
				return filepath.SkipDir
			}
			return nil
		}

		semaphore <- struct{}{}
		go func() {
			defer func() { <-semaphore }()
			img, err := imaging.Open(path)
			if err != nil {
				return
			}

			hash, err := goimagehash.AverageHash(img)
			if err != nil {
				return
			}

			fileInfos <- FileInfo{Path: path, Hash: hash}

			fileCounter++
			if fileCounter >= gcInterval {
				runtime.GC() // Trigger garbage collection
			}
		}()
		return nil
	})

	if err != nil {
		fmt.Printf("Error walking directory: %v\n", err)
	}
}

func isBlacklisted(path string) bool {
	for _, item := range blacklist {
		if strings.Contains(path, item) {
			return true
		}
	}
	return false
}
func compareFiles(fileInfos <-chan FileInfo) {
	processed := make(map[string]*goimagehash.ImageHash)
	processedFolders := make(map[string]map[string]bool)

	// Create the CSV file and write the headers
	headers := []string{"filePath1", "filePath2", "similarity"}
	csv_helper.CreateCSVFileWithHeaders(outputFile, headers)

	for fileInfo := range fileInfos {
		fileDir := filepath.Dir(fileInfo.Path)

		for path, hash := range processed {
			otherFileDir := filepath.Dir(path)
			if fileDir == otherFileDir {
				continue
			}

			if _, ok := processedFolders[fileDir]; ok {
				if _, ok := processedFolders[fileDir][otherFileDir]; ok {
					continue
				}
			}

			distance, err := fileInfo.Hash.Distance(hash)
			if err != nil {
				fmt.Printf("Error calculating hash distance: %v\n", err)
				continue
			}

			similarity := 100 - distance
			if similarity >= similarityThreshold {
				fmt.Printf("Found similar files:\n%s\n%s\nSimilarity: %d%%\n", path, fileInfo.Path, similarity)
				result := []string{path, fileInfo.Path, fmt.Sprintf("%d%%", similarity)}

				csv_helper.AppendResultToCSV(outputFile, result)

				if _, ok := processedFolders[fileDir]; !ok {
					processedFolders[fileDir] = make(map[string]bool)
				}
				processedFolders[fileDir][otherFileDir] = true
			}
		}

		processed[fileInfo.Path] = fileInfo.Hash
	}
}

func createCSVFileWithHeaders(filename string) {
	file, err := os.Create(filename)
	if err != nil {
		fmt.Printf("Error creating CSV file: %v\n", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	headers := []string{"filePath1", "filePath2", "similarity"}
	err = writer.Write(headers)
	if err != nil {
		fmt.Printf("Error writing CSV header: %v\n", err)
		return
	}
}

func appendResultToCSV(filename string, record []string) {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Error opening CSV file for appending: %v\n", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	err = writer.Write(record)
	if err != nil {
		fmt.Printf("Error writing CSV record: %v\n", err)
		return
	}
}
