package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mattanapol/image_manager/internal/common"
	"github.com/mattanapol/image_manager/internal/csv_helper"
	"github.com/mattanapol/image_manager/internal/file_helper"
	ffprobe "gopkg.in/vansante/go-ffprobe.v2"
)

type VideoMetadata struct {
	Width    int
	Height   int
	Bitrate  int
	SizeMb   float64
	Duration string
}

const (
	rootPath   = "/Volumes/CRUCIALSSD"
	outputPath = "./output.csv"
)

func main() {
	var folderPath string
	if len(os.Args) != 2 {
		folderPath = rootPath
	} else {
		folderPath = os.Args[1]
	}

	videos, err := getAllVideos(folderPath)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	headers := []string{"videoPath", "resolution", "fileSize", "duration", "bitrate", "isLarge"}
	csv_helper.CreateCSVFileWithHeaders(outputPath, headers)

	for _, video := range videos {
		metadata, err := getVideoMetadata(video)
		if err != nil {
			// get file size
			fileStat, err := file_helper.GetFileStat(video)
			var fileSize float64 = 0
			if err != nil {
				fmt.Printf("[Error] FileSize: %s\n", err)
				fileSize = float64(fileStat.Size()) / 1024 / 1024
			}

			record := []string{video, "error", fmt.Sprintf("%f", fileSize), "error", "error", "error"}
			csv_helper.AppendResultToCSV(outputPath, record)

			continue
		}

		resolution := fmt.Sprintf("%dx%d", metadata.Width, metadata.Height)
		isLarge := isBitrateLarge(resolution, metadata.Bitrate)

		record := []string{video, resolution, fmt.Sprintf("%f", metadata.SizeMb), metadata.Duration, fmt.Sprintf("%d", metadata.Bitrate), fmt.Sprintf("%t", isLarge)}
		csv_helper.AppendResultToCSV(outputPath, record)
	}

	fmt.Println("Output saved to output.csv")
}

func getVideoMetadata(videoPath string) (VideoMetadata, error) {
	data, err := ffprobe.ProbeURL(context.Background(), videoPath)
	if err != nil {
		return VideoMetadata{}, err
	}

	videoStream := data.FirstVideoStream()
	if videoStream == nil {
		return VideoMetadata{}, errors.New("no video stream found")
	}

	// parse bitrate string to int
	bitrate, err := strconv.Atoi(videoStream.BitRate)
	if err != nil {
		fmt.Printf("[Error] Bitrate: %s\n", videoStream.BitRate)
		bitrate = 0
	}

	// parse file size string to float
	fileSize, err := strconv.ParseFloat(data.Format.Size, 64)
	if err != nil {
		fmt.Printf("[Error] File size: %s\n", data.Format.Size)
		bitrate = 0
	}

	// parse duration string to float
	durationString := videoStream.Duration
	durationFloat, err := strconv.ParseFloat(videoStream.Duration, 64)
	if err == nil {
		var duration time.Duration = time.Duration(durationFloat) * time.Second
		durationString = duration.String()
	}

	metadata := VideoMetadata{
		Width:    videoStream.Width,
		Height:   videoStream.Height,
		Bitrate:  bitrate,
		SizeMb:   float64(fileSize) / 1000000,
		Duration: durationString,
	}

	return metadata, nil
}

func getAllVideos(folder string) ([]string, error) {
	var videos []string
	err := filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() && common.ShouldSkipFolder(path) {
			return filepath.SkipDir
		}

		if !info.IsDir() && isVideo(path) {
			videos = append(videos, path)
		}

		return nil
	})

	return videos, err
}

func isVideo(filename string) bool {
	videoExtensions := []string{".mp4", ".mkv", ".avi", ".mov", ".flv", ".wmv"}
	ext := strings.ToLower(filepath.Ext(filename))
	for _, videoExt := range videoExtensions {
		if ext == videoExt {
			return true
		}
	}

	return false
}

func isBitrateLarge(resolution string, bitrate int) bool {
	var threshold int

	switch resolution {
	case "1920x1080":
		threshold = 6000000
	case "1280x720":
		threshold = 3500000
	case "720x480", "720x576":
		threshold = 2000000
	default:
		threshold = 1000000
	}

	return bitrate > threshold
}
