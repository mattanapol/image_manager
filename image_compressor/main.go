package main

import (
	"fmt"
	"image"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/h2non/filetype"
	"github.com/nfnt/resize"
)

func main() {
	folderPath := "/Volumes/SSD02/temp2/TMW0rzd23bGR48"
	thresholdSize := int64(800000)
	minResolution := uint(2000)
	outputPostfix := "_resized"
	enableResize := true
	defaultScale := 0.8
	jpegQuality := 70
	concurrency := 6

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
		if err != nil {
			return err
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

		*processedFiles = append(*processedFiles, path)
	}
	return nil
}
