package storage

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/minio/minio-go/v7"
	"gitlab.com/transcodeuz/video-transcoder/config"
	"gitlab.com/transcodeuz/video-transcoder/models"
	"gitlab.com/transcodeuz/video-transcoder/pkg/logger"
)

type fileWalkMinio chan string

func (f fileWalkMinio) Walk(path string, info os.FileInfo, err error) error {
	//fmt.Println("--------------------------- ", path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		f <- path
	}
	return nil
}

type MinioStorage struct {
	cfg         *config.Config
	log         logger.Logger
	minioClient *minio.Client
}

// NewMinioStorage ...
func NewMinioStorage(cfg *config.Config, log logger.Logger, minioClient *minio.Client) *MinioStorage {
	return &MinioStorage{
		cfg:         cfg,
		log:         log,
		minioClient: minioClient,
	}
}

func (s *MinioStorage) UploadToCloud(filepath string, pipeline *models.Pipeline) error {
	s.log.Info("[UPLOADING] ", logger.String("filepath", filepath), logger.String("key", pipeline.OutputKey))

	contentType, err := getFileContentType(filepath)
	if err != nil {
		s.log.Error("Error while getting file content type.")
		return err
	}
	res, err := s.minioClient.FPutObject(context.Background(), pipeline.CdnBucket, fmt.Sprintf("%s/%s", pipeline.OutputPath, pipeline.OutputKey), filepath, minio.PutObjectOptions{
		ContentType: contentType,
	})

	if err != nil {
		s.log.Error("Error while uploading to Minio")
		return err
	}

	s.log.Info("Object is uploaded", logger.Any("response", res))
	return nil
}

func (s *MinioStorage) UploadFilesToCloud(pathArg string, p *models.Pipeline) error {
	s.log.Info("[UPLOADING] to minio", logger.String("filepath", pathArg), logger.String("key", p.OutputKey))

	walker := make(fileWalkMinio)
	go func() {
		defer close(walker)

		// Gather the files to upload by walking the path recursively
		if err := filepath.Walk(pathArg, walker.Walk); err != nil {
			s.log.Error("Walk failed:", logger.Error(err))
			return
		}
	}()

	for path := range walker {
		rel, err := filepath.Rel(pathArg, path)
		if err != nil {
			s.log.Error("Unable to get relative path:", logger.Error(err))
			return err
		}
		s.log.Info(path)
		contentType, err := getFileContentType(path)
		if err != nil {
			return err
		}
		_, err = s.minioClient.FPutObject(context.Background(), p.CdnBucket, filepath.Join(p.OutputKey, rel), path, minio.PutObjectOptions{
			ContentType: contentType,
		})
		if err != nil {
			s.log.Error("Failed to upload", logger.Error(err))
			return err
		}
	}

	return nil
}

func getFileContentType(path string) (string, error) {
	// Open File
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Only the first 512 bytes are used to sniff the content type.
	buffer := make([]byte, 512)

	_, err = f.Read(buffer)
	if err != nil {
		return "", err
	}

	// Use the net/http package's handy DectectContentType function. Always returns a valid
	// content-type by returning "application/octet-stream" if no others seemed to match.
	contentType := http.DetectContentType(buffer)

	return contentType, nil
}
