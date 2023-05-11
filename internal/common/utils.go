package common

import "strings"

var (
	skipFolderList = []string{"$RECYCLE.BIN", ".Spotlight", ".fseventsd"}
)

func ShouldSkipFolder(path string) bool {
	for _, item := range skipFolderList {
		if strings.Contains(path, item) {
			return true
		}
	}
	return false
}
