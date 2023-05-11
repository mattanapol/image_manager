package csv_helper

import (
	"encoding/csv"
	"fmt"
	"os"
	"sync"
)

var csvMutex = &sync.Mutex{}

func CreateCSVFileWithHeaders(filename string, headers []string) {
	file, err := os.Create(filename)
	if err != nil {
		fmt.Printf("Error creating CSV file: %v\n", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// headers := []string{"filePath1", "filePath2", "similarity"}
	err = writer.Write(headers)
	if err != nil {
		fmt.Printf("Error writing CSV header: %v\n", err)
		return
	}
}

func AppendResultToCSV(filename string, record []string) {
	csvMutex.Lock()
	defer csvMutex.Unlock()
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
