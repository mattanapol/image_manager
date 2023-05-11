package file_helper

import "os"

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
