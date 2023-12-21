package file_helper

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func GetFileStat(path string) (os.FileInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	head := make([]byte, 261)
	file.Read(head)

	return file.Stat()
}

func GetNextAvailableFilePath(filePath string) (string, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return filePath, nil
	}
	dir := filepath.Dir(filePath)
	base := filepath.Base(filePath)
	ext := filepath.Ext(filePath)
	name := strings.TrimSuffix(base, ext)

	for i := 1; ; i++ {
		newPath := filepath.Join(dir, fmt.Sprintf("%s_%d%s", name, i, ext))
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath, nil
		} else if err != nil {
			return "", err
		}
	}
}
