package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"gitlab.com/transcodeuz/video-transcoder/config"
	"gitlab.com/transcodeuz/video-transcoder/models"
	"gitlab.com/transcodeuz/video-transcoder/pkg/logger"
)

type fileWalk chan string

func (f fileWalk) Walk(path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}
	if !info.IsDir() {
		f <- path
	}
	return nil
}

type S3Storage struct {
	cfg     *config.Config
	log     logger.Logger
	session *session.Session
}

// NewS3Storage ...
func NewS3Storage(cfg *config.Config, log logger.Logger, session *session.Session) *S3Storage {
	return &S3Storage{
		cfg:     cfg,
		log:     log,
		session: session,
	}
}

func (s *S3Storage) UploadToCloud(path string, pipeline *models.Pipeline) error {
	s.log.Info("[UPLOADING] ", logger.String("filepath", path), logger.String("key", pipeline.OutputKey))

	up, err := os.Open(path)
	if err != nil {
		s.log.Error("Error while opening the path", logger.Error(err))
		return err
	}

	uploader := s3manager.NewUploader(s.session)
	res, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(pipeline.CdnBucket),
		Key:    aws.String(pipeline.OutputKey),
		Body:   up,
	})

	if err != nil {
		s.log.Error("Error while uploading the path to S3 bucket", logger.Error(err))
		return err
	}

	s.log.Info("Object is uploaded", logger.Any("response", res))
	return nil
}

func (s *S3Storage) UploadFilesToCloud(pathArg string, pipeline *models.Pipeline) error {
	s.log.Info("[UPLOADING] ", logger.String("filepath", pathArg), logger.String("key", pipeline.OutputKey))

	walker := make(fileWalk)
	go func() {
		defer close(walker)
		// Gather the files to upload by walking the path recursively
		if err := filepath.Walk(pathArg, walker.Walk); err != nil {
			s.log.Error("error while updating walking through path", logger.Error(err))
			return
		}
	}()

	uploader := s3manager.NewUploader(s.session)
	count := 0
	size := 0.0
	for path := range walker {
		if count == 1000 {
			uploader = s3manager.NewUploader(s.session)
			time.Sleep(1 * time.Second)
		}
		count++

		fmt.Println("path", path)
		rel, err := filepath.Rel(pathArg, path)
		if err != nil {
			s.log.Error("unable to get relative path: "+pathArg, logger.Error(err))
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			s.log.Error("error while opening file", logger.Error(err))
			return err
		}

		fileInfo, err := file.Stat()
		if err != nil {
			fmt.Println("Error getting file information:", err)
			return err
		}

		size += float64(fileInfo.Size()) / (1024 * 1024)
		fmt.Println("size", size)
		_, err = uploader.Upload(&s3manager.UploadInput{
			Bucket: aws.String(pipeline.CdnBucket),
			Key:    aws.String(filepath.Join(pipeline.OutputKey, rel)),
			Body:   file,
		})
		if err != nil {
			retryCount := 5
			for i := 0; i < retryCount; i++ {
				_, err = uploader.Upload(&s3manager.UploadInput{
					Bucket: aws.String(pipeline.CdnBucket),
					Key:    aws.String(filepath.Join(pipeline.OutputKey, rel)),
					Body:   file,
				})
				if err == nil {
					break
				}
				time.Sleep(5 * time.Second)
			}
		}
		// close file after uploading to CDN.
		file.Close()

		if err != nil {
			s.log.Error("errr while upload to amazon s3", logger.Error(err))
			return err
		}
	}

	return nil
}
