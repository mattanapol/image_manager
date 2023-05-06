package main

import (
	"fmt"
	"image"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/h2non/filetype"
	"github.com/nfnt/resize"
)

func main() {
	// Configure your settings here
	folderPath := "/Volumes/SSD02/untitled folder/pure media"
	thresholdSize := int64(500000) // 200KB
	minResolution := uint(400)
	outputPostfix := "_resized"
	enableResize := true
	defaultScale := 0.8
	jpegQuality := 70

	var processedFiles []string

	err := filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
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

			if strings.Contains(kind.MIME.Type, "image") && info.Size() > thresholdSize {
				file.Seek(0, 0)
				img, _, err := image.Decode(file)
				if err != nil {
					fmt.Println("Error image decode:", err)
					return err
				}

				width, height := img.Bounds().Dx(), img.Bounds().Dy()
				newHeight := uint(math.Max(float64(height)*defaultScale, float64(minResolution)))
				if enableResize && newHeight < uint(height) {
					originalRatio := float64(width) / float64(height)
					newWidth := uint(originalRatio * float64(newHeight))
					img = resize.Resize(newWidth, newHeight, img, resize.Lanczos3)
				}

				fileName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(filepath.Base(path)))
				outputPath := filepath.Join(filepath.Dir(path), fileName+outputPostfix+".jpg")
				out, err := os.Create(outputPath)
				if err != nil {
					return err
				}
				defer out.Close()

				jpeg.Encode(out, img, &jpeg.Options{Quality: jpegQuality})
				processedFiles = append(processedFiles, path)
			}
		}
		return nil
	})

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
