package config

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cast"
)

type Config struct {
	LogLevel         string
	RabbitMqHost     string
	RabbitMqPort     string
	RabbitMqUser     string
	RabbitMqPassword string
	ListenQueue      string
	WriteQueue       string
	TranscodeWorkers int
	UploadWorkers    int
	Resolutions      []string
	TempFolderPath   string
	TempInputPath    string
	FFmpeg           string
	FFprobe          string
	UseGPU           bool
	HlsTime          string
	Stages           struct {
		Preparation string
		Transcode   string
		Upload      string
	}
	Status struct {
		Pending string
		Success string
		Fail    string
	}
}

func Load() Config {
	err := godotenv.Load(".env")
	if err != nil {
		log.Println("Could not load the .env file")
	}

	c := Config{}
	c.LogLevel = cast.ToString(getOrReturnDefault("LOG_LEVEL", "debug"))

	c.TempFolderPath = cast.ToString(getOrReturnDefault("TEMP_FOLDER_PATH", "transcode"))
	c.TempInputPath = cast.ToString(getOrReturnDefault("TEMP_INPUT_PATH", "transcode-input"))

	c.RabbitMqHost = cast.ToString(getOrReturnDefault("RABBITMQ_HOST", "localhost"))
	c.RabbitMqPort = cast.ToString(getOrReturnDefault("RABBITMQ_PORT", "5672"))
	c.RabbitMqUser = cast.ToString(getOrReturnDefault("RABBITMQ_USER", "user"))
	c.RabbitMqPassword = cast.ToString(getOrReturnDefault("RABBITMQ_PASSWORD", "secret"))

	c.ListenQueue = cast.ToString(getOrReturnDefault("LISTEN_QUEUE", "pipelines"))
	c.WriteQueue = cast.ToString(getOrReturnDefault("WRITE_QUEUE", "pipeline_status"))

	c.TranscodeWorkers = cast.ToInt(getOrReturnDefault("TRANSCODER_WORKERS", 1))
	c.UploadWorkers = cast.ToInt(getOrReturnDefault("UPLOAD_WORKERS", 1))

	c.Resolutions = []string{"240p", "360p", "480p", "720p", "1080p", "4k"}

	c.HlsTime = cast.ToString(getOrReturnDefault("HLS_TIME", "10"))
	c.FFmpeg = cast.ToString(getOrReturnDefault("FFMPEG", "ffmpeg"))
	c.FFprobe = cast.ToString(getOrReturnDefault("FFPROBE", "ffprobe"))
	c.UseGPU = cast.ToBool(getOrReturnDefault("USE_GPU", false))

	c.Stages = struct {
		Preparation string
		Transcode   string
		Upload      string
	}{
		Preparation: "preparation",
		Transcode:   "transcode",
		Upload:      "upload",
	}

	c.Status = struct {
		Pending string
		Success string
		Fail    string
	}{
		Pending: "pending",
		Success: "success",
		Fail:    "fail",
	}

	fmt.Printf("Config-> %+v", c)
	return c
}

func getOrReturnDefault(key string, defaultValue interface{}) interface{} {
	_, exists := os.LookupEnv(key)
	if exists {
		return os.Getenv(key)
	}

	return defaultValue
}
