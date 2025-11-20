package storage

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/minio/minio-go/v7"
	minioCredentials "github.com/minio/minio-go/v7/pkg/credentials"
	"gitlab.com/transcodeuz/video-transcoder/config"
	"gitlab.com/transcodeuz/video-transcoder/models"
	"gitlab.com/transcodeuz/video-transcoder/pkg/logger"
)

// Main Cloud Interface
type MainCloudI interface {
	Minio() *MinioStorage
	S3() *S3Storage
}

// CloudOperationsI - possible actions with storage
type CloudOperationsI interface {
	UploadFilesToCloud(filepath string, pipeline *models.Pipeline) error
}

type cloudObj struct {
	minio *MinioStorage
	s3    *S3Storage
}

// NewStorage - ...
func NewCloudStorage(cfg *config.Config, dynCfg *models.CloudStorageConfig, log logger.Logger) (MainCloudI, error) {
	switch dynCfg.Type {
	case "minio":
		minioClient, err := minio.New(dynCfg.Endpoint, &minio.Options{
			Creds:  minioCredentials.NewStaticV4(dynCfg.AccessKey, dynCfg.SecretKey, ""),
			Secure: true,
		})

		if err != nil {
			log.Error("Error while creating minio client: ", logger.Error(err))
			return nil, err
		}

		return &cloudObj{
			minio: NewMinioStorage(cfg, log, minioClient),
		}, nil
	case "s3":
		awsCfg := &aws.Config{
			Region:      aws.String(dynCfg.Region),
			Credentials: credentials.NewStaticCredentials(dynCfg.AccessKey, dynCfg.SecretKey, ""),
		}
		if dynCfg.Endpoint != "" {
			awsCfg.Endpoint = &dynCfg.Endpoint
		}
		sess, err := session.NewSession(awsCfg)
		if err != nil {
			log.Error("Error while creating aws session: ", logger.Error(err))
			return nil, err
		}

		return &cloudObj{
			s3: NewS3Storage(cfg, log, sess),
		}, nil
	}

	return &cloudObj{}, nil
}

func (s *cloudObj) Minio() *MinioStorage {
	return s.minio
}

func (s *cloudObj) S3() *S3Storage {
	return s.s3
}
