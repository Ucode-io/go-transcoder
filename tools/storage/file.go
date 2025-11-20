package storage

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"gitlab.com/transcodeuz/video-transcoder/config"
	"gitlab.com/transcodeuz/video-transcoder/pkg/logger"
)

type fileStorage struct {
	log logger.Logger
	cfg *config.Config
}

type FileOperationsI interface {
	RemoveFromDir(filePath string) error
	GetOutputPath(key string) string
	CreateFolder(key string) error
	GetUploadPath(key string) string
	DownloadWithWget(url, filePath string) error
	ReadFileLines(filename string) ([]string, error)
	WriteLinesToFile(lines []string, filename string) error
}

func NewFileStorage(cfg *config.Config, log logger.Logger) FileOperationsI {
	return &fileStorage{
		cfg: cfg,
		log: log,
	}
}

func (f *fileStorage) RemoveFromDir(filePath string) error {
	f.log.Info("Removing from directory", logger.String("info", filePath))
	if len(filePath) > 0 {
		if filePath[len(filePath)-1] == '/' {
			filePath = string(filePath[:len(filePath)-1])
		}
	}
	fmt.Println("remove func statrted last")
	err := os.RemoveAll(filePath)
	return err
}

func (f *fileStorage) GetOutputPath(key string) string {
	names := strings.Split(key, ".")

	return fmt.Sprintf("/%s/%s", f.cfg.TempFolderPath, names[0])
}

func (f *fileStorage) GetUploadPath(key string) string {
	names := strings.Split(key, ".")
	return fmt.Sprintf("/%s/%s/", f.cfg.TempFolderPath, names[0])
}

func (f *fileStorage) CreateFolder(key string) error {
	resolutions := []string{"240p", "360p", "480p", "720p", "1080p", "4k", "audio", "subtitle"}

	if _, err := os.Stat(fmt.Sprintf("%s/%s", f.cfg.TempFolderPath, key)); os.IsNotExist(err) {
		err := os.Mkdir(fmt.Sprintf("%s/%s", f.cfg.TempFolderPath, key), 0755)
		if err != nil {
			f.log.Error("Error while creating the directory", logger.Error(err))
			return err
		}
	}

	for _, res := range resolutions {
		if _, err := os.Stat(fmt.Sprintf("%s/%s/%s", f.cfg.TempFolderPath, key, res)); os.IsNotExist(err) {
			err := os.Mkdir(fmt.Sprintf("%s/%s/%s", f.cfg.TempFolderPath, key, res), 0755)
			if err != nil {
				f.log.Error("Error while creating the directory", logger.Error(err))
				return err
			}
		}
	}

	return nil
}

func (f *fileStorage) DownloadWithWget(url, filePath string) error {
	_, err := exec.Command("wget", "-O", filePath, url).CombinedOutput()
	if err != nil {
		return fmt.Errorf("error running wget: %s", err)
	}

	return nil
}

func (f *fileStorage) ReadFileLines(filename string) ([]string, error) {
	// Open the file
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Create a scanner to read the file line by line
	scanner := bufio.NewScanner(file)

	var lines []string
	// Read each line and append it to the slice
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	// Check for any errors encountered during scanning
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}

func (f *fileStorage) WriteLinesToFile(lines []string, filename string) error {
	// Concatenate lines with newline separators
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}

	// Write the content to the file
	err := os.WriteFile(filename, []byte(content), 0644)
	if err != nil {
		return err
	}

	return nil
}
