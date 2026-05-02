package fm

import (
	"fmt"
	"os"
	"path/filepath"
)

func GetFileList(directory string) ([]os.FileInfo, error) {
	var files []os.FileInfo
	err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, info)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("无法遍历目录 %s: %v", directory, err)
	}
	return files, nil
}
