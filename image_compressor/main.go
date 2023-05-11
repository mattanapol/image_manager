package main

import (
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/h2non/filetype"
	"github.com/nfnt/resize"
	"golang.org/x/exp/slices"
)

var (
	skipFolderList         = []string{"$RECYCLE.BIN", ".Spotlight", ".fseventsd"}
	unwantedFileExtensions = []string{".url", ".download", ".js", ".css", ".html", ".ass", ".php", ".txt"}
	processFileCount       = 0
	countMutex             sync.Mutex
)

func main() {
	folderPath := "/Volumes/CRUCIALSSD/temp21"
	thresholdSize := int64(900000)
	minResolution := uint(2000)
	outputPostfix := "_resized"
	enableResize := true
	defaultScale := 0.8
	jpegQuality := 70
	concurrency := 6

	flattenFolder(folderPath)

	var processedFiles []string
	var wg sync.WaitGroup

	fileChan := make(chan string, concurrency)
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			for path := range fileChan {
				err := processFile(path, thresholdSize, minResolution, outputPostfix, enableResize, defaultScale, jpegQuality, &processedFiles)
				if err != nil {
					fmt.Println("Error processing file:", path, "Error:", err)
				}
			}
			wg.Done()
		}()
	}

	err := filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil && !errors.Is(err, os.ErrPermission) {
			return err
		}

		if isBlacklisted(path) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !info.IsDir() {
			fileChan <- path
		}
		return nil
	})

	close(fileChan)
	wg.Wait()

	if err != nil {
		fmt.Println("Error processing files:", err)
		return
	}

	fmt.Println("Processed", processFileCount, "files.")
	fmt.Print("Do you want to delete the original image files that were processed? (Y/N): ")
	var input string
	fmt.Scanln(&input)

	if strings.ToLower(input) == "y" {
		for _, path := range processedFiles {
			err := os.Remove(path)
			if err != nil {
				fmt.Println("Error deleting file:", path, "Error:", err)
			}
		}
		fmt.Println("Original image files that were processed have been deleted successfully.")
	}
}

func processFile(path string, thresholdSize int64, minResolution uint, outputPostfix string, enableResize bool, defaultScale float64, jpegQuality int, processedFiles *[]string) error {
	fmt.Println("Processing file:", path)
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	head := make([]byte, 261)
	file.Read(head)
	kind, _ := filetype.Match(head)
	if kind == filetype.Unknown {
		return nil
	}

	if strings.Contains(kind.MIME.Type, "image") {
		fileInfo, err := file.Stat()
		if err != nil {
			return err
		}

		if fileInfo.Size() <= thresholdSize {
			return nil
		}
		file.Seek(0, 0)
		img, _, err := image.Decode(file)
		if err != nil {
			fmt.Println("Error decoding image config:", err)
			return err
		}

		width, height := img.Bounds().Dx(), img.Bounds().Dy()
		newHeight := uint(math.Max(float64(height)*defaultScale, float64(minResolution)))
		if enableResize && newHeight < uint(height) {
			originalRatio := float64(width) / float64(height)
			newWidth := uint(originalRatio * float64(newHeight))

			file.Seek(0, 0)
			if err != nil {
				fmt.Println("Error image decode:", err)
				return err
			}

			img = resize.Resize(newWidth, newHeight, img, resize.Lanczos3)
		}

		fileName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(filepath.Base(path)))
		outputPath := filepath.Join(filepath.Dir(path), fileName+outputPostfix+".jpg")
		out, err := os.Create(outputPath)
		if err != nil {
			return err
		}
		defer out.Close()

		err = jpeg.Encode(out, img, &jpeg.Options{Quality: jpegQuality})
		if err != nil {
			return err
		}

		err = os.Remove(path)
		if err != nil {
			fmt.Println("Error deleting file:", path, "Error:", err)
		}
		countMutex.Lock()
		processFileCount++
		countMutex.Unlock()

		// *processedFiles = append(*processedFiles, path)
	}
	return nil
}

func isBlacklisted(path string) bool {
	for _, item := range skipFolderList {
		if strings.Contains(path, item) {
			return true
		}
	}
	return false
}

func flattenFolder(parentFolder string) error {
	if isBlacklisted(parentFolder) {
		return nil
	}
	clearUnwantedFiles(parentFolder)
	// Get a list of all the items in the parent folder
	items, err := os.ReadDir(parentFolder)
	if err != nil {
		return err
	}

	if len(items) == 0 {
		os.Remove(parentFolder)
		return filepath.SkipDir
	}

	// Check if there's only one item in the parent folder, and if it's a directory
	if len(items) == 1 && items[0].IsDir() {
		// Get the path to the subfolder
		subfolderPath := filepath.Join(parentFolder, items[0].Name())

		// Move all the files in the subfolder to the parent folder
		subfolderItems, err := os.ReadDir(subfolderPath)
		if err != nil {
			return err
		}
		for _, file := range subfolderItems {
			filePath := filepath.Join(subfolderPath, file.Name())
			err = os.Rename(filePath, filepath.Join(parentFolder, file.Name()))
			if err != nil {
				return err
			}
		}

		// Delete the subfolder
		err = os.Remove(subfolderPath)
		if err != nil {
			return err
		}
	} else {
		for _, item := range items {
			if item.IsDir() {
				err = flattenFolder(filepath.Join(parentFolder, item.Name()))
				if errors.Is(err, filepath.SkipDir) {
					continue
				}
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func clearUnwantedFiles(parentFolder string) error {
	if isBlacklisted(parentFolder) {
		return nil
	}

	// Get a list of all the items in the parent folder
	items, err := os.ReadDir(parentFolder)
	if err != nil {
		return err
	}

	for _, item := range items {
		if item.IsDir() {
			continue
		}
		// if file extension match unwanted list, delete it
		if slices.Contains(unwantedFileExtensions, strings.ToLower(filepath.Ext(item.Name()))) {
			err = os.Remove(filepath.Join(parentFolder, item.Name()))
			if err != nil {
				return err
			}
		}
	}

	return nil
}
