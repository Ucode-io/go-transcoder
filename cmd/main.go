package main

import (
	"context"

	"gitlab.com/transcodeuz/video-transcoder/config"
	"gitlab.com/transcodeuz/video-transcoder/pkg/handler"
	"gitlab.com/transcodeuz/video-transcoder/pkg/logger"
	"gitlab.com/transcodeuz/video-transcoder/pkg/rabbitmq"
	"gitlab.com/transcodeuz/video-transcoder/tools/ffmpeg"
	"gitlab.com/transcodeuz/video-transcoder/tools/storage"
)

func main() {
	cfg := config.Load()
	log := logger.New(cfg.LogLevel, "transcoder_service_new")

	log.Info("new configuration and logger is setup...")

	rbMQ, err := rabbitmq.New(&cfg, log)
	if err != nil {
		log.Error("Error while creating rabbitMq object...", logger.Error(err))
		return
	}

	// We need to close the channel if we have opened it
	defer rbMQ.Channel.Close()

	fileStorage := storage.NewFileStorage(&cfg, log)
	log.Info("storage is created...")

	transcoder := ffmpeg.NewFFmpeg(&cfg, log)
	log.Info("transcoder is created...")

	handlerObj := handler.NewHandler(handler.Options{
		Config:       &cfg,
		Log:          log,
		LocalStorage: fileStorage,
		Transcoder:   transcoder,
		RabbitMQ:     rbMQ,
	})

	handlerObj.ListenNotifications(context.Background())
}
