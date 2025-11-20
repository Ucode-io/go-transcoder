package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"gitlab.com/transcodeuz/video-transcoder/tools/subtitle"
	"os"
	"strconv"
	"strings"
	"time"

	"gitlab.com/transcodeuz/video-transcoder/config"
	"gitlab.com/transcodeuz/video-transcoder/models"
	"gitlab.com/transcodeuz/video-transcoder/pkg/logger"
	"gitlab.com/transcodeuz/video-transcoder/pkg/rabbitmq"
	"gitlab.com/transcodeuz/video-transcoder/tools/storage"
	"gitlab.com/transcodeuz/video-transcoder/tools/transcoder"

	"github.com/streadway/amqp"
)

// Job is the structure which added to the queue
type Job struct {
	data amqp.Delivery
}

// Options ...
type Options struct {
	Config       *config.Config
	Log          logger.Logger
	CloudStorage storage.MainCloudI
	LocalStorage storage.FileOperationsI
	Transcoder   transcoder.Transcoder
	RabbitMQ     *rabbitmq.RabbitMQ
}

// MainI - interface containing main functions for handler
type MainI interface {
	ListenNotifications(ctx context.Context) error
}

type handlerObj struct {
	cfg              *config.Config
	log              logger.Logger
	transcoder       transcoder.Transcoder
	CloudStorage     storage.MainCloudI
	LocalStorage     storage.FileOperationsI
	rabbitMQ         *rabbitmq.RabbitMQ
	preparationQueue chan Job
	videoQueue       chan transcoder.TrInfo
	fileQueue        chan transcoder.TrInfo
}

// NewHandler - returns the handler object
func NewHandler(args Options) MainI {
	return &handlerObj{
		cfg:              args.Config,
		log:              args.Log,
		transcoder:       args.Transcoder,
		rabbitMQ:         args.RabbitMQ,
		CloudStorage:     args.CloudStorage,
		LocalStorage:     args.LocalStorage,
		preparationQueue: make(chan Job, args.Config.TranscodeWorkers),
		videoQueue:       make(chan transcoder.TrInfo, args.Config.TranscodeWorkers),
		fileQueue:        make(chan transcoder.TrInfo, args.Config.UploadWorkers),
	}
}

func (h *handlerObj) ListenNotifications(ctx context.Context) error {
	for i := 0; i < h.cfg.TranscodeWorkers; i++ {
		go h.PreparationWorker(i)
		go h.TranscodeWorker(i)
	}

	for i := 0; i < h.cfg.UploadWorkers; i++ {
		go h.CloudStorageWorker(i)
	}

	h.log.Info("Started listening for notifications")

	for {
		msgs, err := h.rabbitMQ.Channel.Consume(
			h.rabbitMQ.Queues[h.cfg.ListenQueue].Name,
			"",
			false,
			false,
			false,
			false,
			nil,
		)

		if err != nil {
			h.log.Error("Error while consuming messages", logger.Error(err))
			h.log.Info("Sleeping one second...")
			err = h.rabbitMQ.Reconnect()
			if err != nil {
				panic("couldn't reconnect to rabbitmq")
			} else {
				time.Sleep(time.Second * 5)
				continue
			}
		}

		h.log.Info("Inside the go routine")
		for data := range msgs {
			h.AddPreparationQueue(Job{data: data})
			data.Ack(false)
		}
		time.Sleep(time.Second * 5)
	}
}

func (h *handlerObj) TranscodeWorker(id int) {
	workerId := "worker[" + strconv.Itoa(id) + "] TRANSCODER"
	h.log.Info(workerId, logger.String("action", "[STARTING]"))

	for job := range h.videoQueue {
		h.log.Info("==================== Message is received ====================================")
		h.log.Info(workerId, logger.String("action", "[GET]"), logger.String("message[key]", job.Output))
		h.Transcode(job)
	}
}

func (h *handlerObj) PreparationWorker(id int) {
	workerId := "worker[" + strconv.Itoa(id) + "] PREPARATION"
	h.log.Info(workerId, logger.String("action", "[STARTING]"))

	for job := range h.preparationQueue {
		msg := &models.Pipeline{}
		err := json.Unmarshal(job.data.Body, &msg)
		if err != nil {
			h.log.Error("[-] UNMARSHAL", logger.Error(err))
			continue
		}

		h.log.Info("==================== Message is received ====================================")
		h.log.Info(workerId, logger.String("action", "[GET]"), logger.String("message[key]", msg.OutputKey))

		h.Prepare(msg)
	}
}

func (h *handlerObj) Prepare(pipeline *models.Pipeline) {
	var (
		videoInfo      *transcoder.VideoInfo
		err            error
		transcoderInfo = transcoder.TrInfo{
			FileName: pipeline.OutputKey,
			Input:    pipeline.InputURI,
			Output:   pipeline.OutputKey,
			Pipeline: pipeline,
			UseGPU:   h.cfg.UseGPU,
		}
	)
	updatePipelineReq := &models.UpdatePipelineStage{
		Id:        pipeline.Id,
		Stage:     h.cfg.Stages.Preparation,
		Status:    h.cfg.Status.Pending,
		ErrorCode: Success,
	}

	err = h.rabbitMQ.PublishPipelineStatus(updatePipelineReq)
	if err != nil {
		h.log.Error("Error while publishing to rabbit mq.", logger.Error(err))
	}

	err = h.LocalStorage.CreateFolder(transcoderInfo.Pipeline.OutputKey)
	if err != nil {
		h.log.Error("Error while creating directory", logger.Error(err))
		updatePipelineReq.ErrorCode = InternalServerError
		updatePipelineReq.Status = h.cfg.Status.Fail
		updatePipelineReq.FailDescription = err.Error()
		err = h.rabbitMQ.PublishPipelineStatus(updatePipelineReq)
		if err != nil {
			h.log.Error("Error while publishing to rabbit mq.", logger.Error(err))
		}
		return
	}

	start := time.Now()

	paths := strings.Split(transcoderInfo.Input, "/")
	newInputPath := fmt.Sprintf("%s/%s", h.cfg.TempInputPath, paths[len(paths)-1])

	fmt.Println("New input path", newInputPath)
	err = h.LocalStorage.DownloadWithWget(transcoderInfo.Input, newInputPath)
	if err != nil {
		updatePipelineReq.ErrorCode = InternalServerError
		updatePipelineReq.Status = h.cfg.Status.Fail
		updatePipelineReq.FailDescription = "Error while downloading video: " + err.Error()
		err = h.rabbitMQ.PublishPipelineStatus(updatePipelineReq)
		if err != nil {
			h.log.Error("Error while publishing to rabbit mq.", logger.Error(err))
		}
		h.log.Error("[-] DOWNLOAD VIDEO", logger.Error(err), logger.String("INPUT", transcoderInfo.Input))
		return
	}
	h.log.Info("[+] DOWNLOAD VIDEO", logger.String("INFO", newInputPath))
	transcoderInfo.Input = newInputPath

	// get video streams info
	videoInfo, err = h.transcoder.GetVideoInfo(transcoderInfo.Input)
	if err != nil {
		updatePipelineReq.ErrorCode = InvalidRequest
		updatePipelineReq.FailDescription = "Couldn't convert the video into a format that we handle. The reason may be corrupted file." + err.Error()
		updatePipelineReq.Status = h.cfg.Status.Fail
		err = h.rabbitMQ.PublishPipelineStatus(updatePipelineReq)
		if err != nil {
			h.log.Error("Error while publishing to rabbit mq.", logger.Error(err))
		}
		return
	}
	transcoderInfo.VideoInfo = *videoInfo

	for _, e := range videoInfo.Streams {
		if e.Duration != "" {
			transcoderInfo.Duration, err = strconv.ParseFloat(e.Duration, 64)
			if err != nil {
				h.log.Error("[-] GET VIDEO DURATION", logger.Error(err), logger.String("INFO", fmt.Sprintf("%v", videoInfo)))

				updatePipelineReq.ErrorCode = InternalServerError
				updatePipelineReq.Status = h.cfg.Status.Fail
				updatePipelineReq.FailDescription = "Error while getting video duration: " + err.Error()

				err = h.rabbitMQ.PublishPipelineStatus(updatePipelineReq)
				if err != nil {
					h.log.Error("Error while publishing to rabbit mq.", logger.Error(err))
				}
				fmt.Println("removie func starterd 2")
				err = os.Remove(newInputPath)
				if err != nil {
					updatePipelineReq.FailDescription = "Error while removign from input local storage: " + err.Error()
					err = h.rabbitMQ.PublishPipelineStatus(updatePipelineReq)
					if err != nil {
						h.log.Error("Error while publishing to rabbit mq.", logger.Error(err))
					}
				}
				return
			}
			break
		}
	}

	h.log.Info("[+] GET VIDEO DURATION", logger.String("DURATION", strconv.FormatFloat(transcoderInfo.Duration, 'E', -1, 64)))

	transcoderInfo.Resolution, err = transcoder.GetResolutionFormat(*videoInfo)
	if err != nil {
		h.log.Error("[-] GET HIGHEST RESOLUTION", logger.Error(err), logger.String("INFO", fmt.Sprintf("%+v", videoInfo)))

		updatePipelineReq.ErrorCode = InternalServerError
		updatePipelineReq.Status = h.cfg.Status.Fail
		updatePipelineReq.FailDescription = "Error while getting video info: " + err.Error()
		err = h.rabbitMQ.PublishPipelineStatus(updatePipelineReq)
		if err != nil {
			h.log.Error("Error while publishing to rabbit mq.", logger.Error(err))
		}
		fmt.Println("remove func started 3")
		err = os.Remove(newInputPath)
		if err != nil {
			updatePipelineReq.FailDescription = "Error while removign from input local storage: " + err.Error()
			err = h.rabbitMQ.PublishPipelineStatus(updatePipelineReq)
			if err != nil {
				h.log.Error("Error while publishing to rabbit mq.", logger.Error(err))
			}
		}

		return
	}
	h.log.Info("[+] GET HIGHEST RESOLUTION", logger.String("RESOLUTION", fmt.Sprintf("%+v", transcoderInfo.Resolution)))

	end := time.Since(start)
	updatePipelineReq.PreparationDuration = int(end.Milliseconds())
	updatePipelineReq.VideoDuration = transcoderInfo.Duration
	updatePipelineReq.Status = h.cfg.Status.Success

	for _, e := range transcoder.ResolutionsPriorityUpdate {
		if e.Priority <= transcoderInfo.Resolution.Priority {
			updatePipelineReq.Resolutions = append(updatePipelineReq.Resolutions, models.Resolution{
				Resolution: e.Resolution,
				Measure:    e.Measure,
				BitRate:    e.VideoBitRate,
			})
		}
	}

	err = h.rabbitMQ.PublishPipelineStatus(updatePipelineReq)
	if err != nil {
		h.log.Error("Error while publishing to rabbit mq.", logger.Error(err))
	}

	transcoderInfo.UpdatePipelineStage = *updatePipelineReq
	h.AddTranscodeQueue(transcoderInfo)
}

func (h *handlerObj) Transcode(transcoderInfo transcoder.TrInfo) {
	updatePipelineReq := &transcoderInfo.UpdatePipelineStage
	updatePipelineReq.Stage = h.cfg.Stages.Transcode
	updatePipelineReq.Status = h.cfg.Status.Pending
	start := time.Now()
	err := h.rabbitMQ.PublishPipelineStatus(updatePipelineReq)
	if err != nil {
		h.log.Error("Error while publishing to rabbit mq.", logger.Error(err))
	}

	defer func() {
		fmt.Println("removie func started 1")
		err = os.Remove(transcoderInfo.Input)
		if err != nil {
			h.log.Error("error while removing field from temp input")
		}
	}()

	transcoderInfo.Output = h.LocalStorage.GetOutputPath(transcoderInfo.Output)

	h.log.Info("Item's outputpath: ", logger.String("filename", transcoderInfo.FileName), logger.String("", transcoderInfo.Output))

	err = h.transcoder.ResizeVideoGpuMaster(transcoderInfo)
	if err != nil {
		updatePipelineReq.ErrorCode = InternalServerError
		updatePipelineReq.Status = h.cfg.Status.Fail
		updatePipelineReq.FailDescription = "Error while Transcoding video: " + err.Error()
		err = h.rabbitMQ.PublishPipelineStatus(updatePipelineReq)
		if err != nil {
			h.log.Error("Error while publishing to rabbit mq.", logger.Error(err))
		}
		h.log.Error("[-] RESIZED the movie", logger.Error(err), logger.String("INFO", fmt.Sprintf("%+v", transcoderInfo.VideoInfo)))
		return
	}
	h.log.Info("[+] RESIZED the movie", logger.String("output-path", transcoderInfo.Output))

	transcoderInfo.UpdatePipelineStage = *updatePipelineReq
	// Add a pipeline to another worker to upload to cloud storage

	err = h.AddAudioTracks(transcoderInfo)
	if err != nil {
		updatePipelineReq.ErrorCode = InternalServerError
		updatePipelineReq.Status = h.cfg.Status.Fail
		updatePipelineReq.FailDescription = "Error while adding multiple audio tracks to the playlist: " + err.Error()
		err = h.rabbitMQ.PublishPipelineStatus(updatePipelineReq)
		if err != nil {
			h.log.Error("Error while publishing to rabbit mq.", logger.Error(err))
		}
		h.log.Error("Error while adding audio tracks", logger.Error(err))
		return
	}

	err = h.AddSubtitles(transcoderInfo)
	if err != nil {
		updatePipelineReq.ErrorCode = InternalServerError
		updatePipelineReq.Status = h.cfg.Status.Fail
		updatePipelineReq.FailDescription = "Error while adding multiple subtitles to the playlist: " + err.Error()
		err = h.rabbitMQ.PublishPipelineStatus(updatePipelineReq)
		if err != nil {
			h.log.Error("Error while publishing to rabbit mq.", logger.Error(err))
		}
		h.log.Error("Error while adding subtitles", logger.Error(err))
		return
	}

	end := time.Since(start)

	updatePipelineReq.TranscodeDuration = int(end.Milliseconds())
	updatePipelineReq.Status = h.cfg.Status.Success
	err = h.rabbitMQ.PublishPipelineStatus(updatePipelineReq)
	if err != nil {
		h.log.Error("Error while publishing to rabbit mq.", logger.Error(err))
	}

	h.AddCloudStorageQueue(transcoderInfo)
}

/*
1. Get the audio track of the orginal video
2. Add audio track of the orginal video to list of audio tracks
3. Create needed folders for the audio tracks
4. Create hls stream for each audio track
5. Add all audio streams to main playlist
6. Remove audio track of the original video
*/
func (h *handlerObj) AddAudioTracks(transcodeInfo transcoder.TrInfo) error {
	// 1. Get the audio track of the orginal video

	streams, err := h.transcoder.GetAudioStreamName(transcodeInfo.Input)
	if err != nil {
		h.log.Error("Error while getting audio streams", logger.Error(err))
		return err
	}
	for _, e := range transcodeInfo.VideoInfo.Streams {
		if e.CodecType == "audio" {
			if _, ok := streams[e.Index]; !ok {
				if e.Tags.Language == "" {
					streams[e.Index] = "rus"
				} else {
					streams[e.Index] = e.Tags.Language
				}
			}
			fmt.Println(streams[e.Index])
			audioTrackInput := fmt.Sprintf("/%s/%s/audio/%s.mp3", h.cfg.TempFolderPath, transcodeInfo.Pipeline.OutputKey, streams[e.Index])
			// err := os.Mkdir(fmt.Sprintf("/%s/%s",  h.cfg.TempFolderPath, transcodeInfo.Pipeline.OutputKey), 0755)
			// if err != nil {
			// 	h.log.Error("Error while createing folder", logger.Error(err))
			// 	return err
			// }
			err := h.transcoder.GetAudioTrack(transcodeInfo.Input, audioTrackInput, e.Index)
			if err != nil {
				return err
			}

			h.log.Info("Successfully get audio track from video", logger.String(audioTrackInput, "message[key]"))
			// 2. Add audio track of the orginal video to list of audio tracks
			transcodeInfo.Pipeline.AudioTracks = append(transcodeInfo.Pipeline.AudioTracks, models.AudioTrack{
				InputURL:     audioTrackInput,
				LanguageCode: e.Tags.Language,
				Language:     streams[e.Index],
			})
		}
	}

	// transcodeInfo.Pipeline.AudioTracks = append([]models.AudioTrack{
	// 	{
	// 		InputURL:     audioTrackInput,
	// 		LanguageCode: transcodeInfo.Pipeline.LanguageCode,
	// 		Language:     transcodeInfo.Pipeline.Language,
	// 	},
	// }, transcodeInfo.Pipeline.AudioTracks...)

	// 3. Create needed folders for the audio tracks
	for _, e := range transcodeInfo.Pipeline.AudioTracks {
		if _, err := os.Stat(fmt.Sprintf("/%s/%s/audio/%s", h.cfg.TempFolderPath, transcodeInfo.Pipeline.OutputKey, e.Language)); os.IsNotExist(err) {
			err := os.Mkdir(fmt.Sprintf("/%s/%s/audio/%s", h.cfg.TempFolderPath, transcodeInfo.Pipeline.OutputKey, e.Language), 0755)
			if err != nil {
				return err
			}
		}
	}

	// 4. Create hls stream for each audio track
	for _, e := range transcodeInfo.Pipeline.AudioTracks {
		err := h.transcoder.CreateAudioStream(e, fmt.Sprintf("/%s/%s/audio/%s/index.m3u8", h.cfg.TempFolderPath, transcodeInfo.Pipeline.OutputKey, e.Language))
		if err != nil {
			return err
		}
	}

	// 5. Add all audio streams to main playlist
	lines := []string{}
	for i, e := range transcodeInfo.Pipeline.AudioTracks {
		isDefault := "NO"
		if i == 0 {
			isDefault = "YES"
		}

		lines = append(lines, fmt.Sprintf(
			"#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"audio\",NAME=\"%s\",DEFAULT=%s,AUTOSELECT=%s,LANGUAGE=\"%s\",URI=\"%s\"",
			e.Language, isDefault, isDefault, e.LanguageCode, fmt.Sprintf("audio/%s/index.m3u8", e.Language),
		))
	}

	temp, err := h.LocalStorage.ReadFileLines(fmt.Sprintf("/%s/%s/master.m3u8", h.cfg.TempFolderPath, transcodeInfo.Pipeline.OutputKey))
	if err != nil {
		return err
	}

	fileLines := []string{}
	fileLines = append(fileLines, temp[:2]...)
	fileLines = append(fileLines, lines...)
	fileLines = append(fileLines, []string{"", ""}...)
	fileLines = append(fileLines, temp[2:]...)

	for i, e := range fileLines {
		if i >= 4 && strings.Contains(e, "RESOLUTION") {
			fileLines[i] = e[:len(e)-1] + ",mp4a.40.2\",AUDIO=\"audio\""
		}
	}

	err = h.LocalStorage.WriteLinesToFile(fileLines, fmt.Sprintf("/%s/%s/master.m3u8", h.cfg.TempFolderPath, transcodeInfo.Pipeline.OutputKey))
	if err != nil {
		return err
	}

	// 6. Remove audio track of the original video
	// for transcodeInfo.Pipeline.AudioTracks != nil {
	// 	fmt.Println("removie func started 4")
	// 	err = os.Remove(transcodeInfo.Pipeline.AudioTracks[0].InputURL)
	// 	if err != nil {
	// 		h.log.Error("Erro while deleting audio file", logger.Error(err))
	// 		return nil
	// 		// return err
	// 	}
	// }
	return nil
}

/*
1. Create needed folders for the subtitle tracks
2. Create playlist stream for each subtitle
3. Add all subtitles to main playlist
*/
func (h *handlerObj) AddSubtitles(transcodeInfo transcoder.TrInfo) error {
	// 1. Extract subtitles from video
	subtitlesName, err := h.transcoder.GetSubtitleName(transcodeInfo.Input)
	if err != nil {
		h.log.Error("Error while getting subtitle streams", logger.Error(err))
		subtitlesName = make(map[int]string)
	}
	for k, e := range transcodeInfo.Pipeline.Subtitle {
		if _, err := os.Stat(fmt.Sprintf("/%s/%s/subtitle/%s", h.cfg.TempFolderPath, transcodeInfo.Pipeline.OutputKey, e.Language)); os.IsNotExist(err) {
			err := os.Mkdir(fmt.Sprintf("/%s/%s/subtitle/%s", h.cfg.TempFolderPath, transcodeInfo.Pipeline.OutputKey, e.Language), 0755)
			if err != nil {
				return err
			}
		}
		folder := fmt.Sprintf("/%s/%s/subtitle/%s", h.cfg.TempFolderPath, transcodeInfo.Pipeline.OutputKey, e.Language)
		vttFile := fmt.Sprintf("%s/%s.vtt", folder, e.Language)
		err := h.transcoder.SubtitleFileTOVTTFile(vttFile, e.InputURL)
		if err != nil {
			h.log.Error("Error while creating subtitles streams", logger.Error(err))
			transcodeInfo.Pipeline.Subtitle = append(transcodeInfo.Pipeline.Subtitle[:k], transcodeInfo.Pipeline.Subtitle[k+1:]...)
			continue
		}
		err = subtitle.CreateM3U8FromVTT(fmt.Sprintf("%s/", folder), e.Language+".vtt")
		if err != nil {
			h.log.Error("Error while extracting subtitle CreateM3U8FromVTT", logger.Error(err))
			transcodeInfo.Pipeline.Subtitle = append(transcodeInfo.Pipeline.Subtitle[:k], transcodeInfo.Pipeline.Subtitle[k+1:]...)
			continue
		}

		transcodeInfo.Pipeline.Subtitle[k] = models.SubtitleRequest{
			InputURL:     vttFile,
			LanguageCode: e.Language,
			Language:     e.Language,
		}
	}

	for _, e := range transcodeInfo.VideoInfo.Streams {
		if e.CodecType == "subtitle" {
			if _, ok := subtitlesName[e.Index]; !ok {
				if e.Tags.Language == "" {
					continue
				} else {
					subtitlesName[e.Index] = e.Tags.Language
				}
			}
			if _, err := os.Stat(fmt.Sprintf("/%s/%s/subtitle/%s", h.cfg.TempFolderPath, transcodeInfo.Pipeline.OutputKey, subtitlesName[e.Index])); os.IsNotExist(err) {
				err := os.Mkdir(fmt.Sprintf("/%s/%s/subtitle/%s", h.cfg.TempFolderPath, transcodeInfo.Pipeline.OutputKey, subtitlesName[e.Index]), 0755)
				if err != nil {
					return err
				}
			}
			folder := fmt.Sprintf("/%s/%s/subtitle/%s", h.cfg.TempFolderPath, transcodeInfo.Pipeline.OutputKey, subtitlesName[e.Index])
			fileURL, err := h.transcoder.ExtractSubtitleStream(transcodeInfo.Input, e.Index, subtitlesName[e.Index], folder)
			if err != nil {
				h.log.Error("Error while extracting subtitle streams", logger.Error(err))
				continue
			}

			err = subtitle.CreateM3U8FromVTT(fmt.Sprintf("/%s/%s/subtitle/%s/", h.cfg.TempFolderPath, transcodeInfo.Pipeline.OutputKey, subtitlesName[e.Index]), subtitlesName[e.Index]+".vtt")
			if err != nil {
				h.log.Error("Error while extracting subtitle CreateM3U8FromVTT", logger.Error(err))
				continue
			}

			transcodeInfo.Pipeline.Subtitle = append(transcodeInfo.Pipeline.Subtitle, models.SubtitleRequest{
				InputURL:     fileURL,
				LanguageCode: e.Tags.Language,
				Language:     subtitlesName[e.Index],
			})
		}
	}

	if len(transcodeInfo.Pipeline.Subtitle) == 0 {
		return nil
	}
	// 1. Create needed folders for the subtitle tracks
	for _, e := range transcodeInfo.Pipeline.Subtitle {
		if _, err := os.Stat(fmt.Sprintf("/%s/%s/subtitle/%s", h.cfg.TempFolderPath, transcodeInfo.Pipeline.OutputKey, e.Language)); os.IsNotExist(err) {
			err := os.Mkdir(fmt.Sprintf("/%s/%s/subtitle/%s", h.cfg.TempFolderPath, transcodeInfo.Pipeline.OutputKey, e.Language), 0755)
			if err != nil {
				return err
			}
		}
	}

	// 2. Create playlist stream for each subtitle
	//for _, e := range transcodeInfo.Pipeline.Subtitle {
	//	err := h.transcoder.CreateSubtitleStreams(e, transcodeInfo.Pipeline.OutputKey)
	//	if err != nil {
	//		h.log.Error("Error while creating subtitles streams", logger.Error(err))
	//		return err
	//	}
	//}

	// 3. Add all subtitles to main playlist
	lines := []string{}
	for i, e := range transcodeInfo.Pipeline.Subtitle {
		isDefault := "NO"
		if i == 0 {
			isDefault = "YES"
		}

		lines = append(lines, fmt.Sprintf(
			"#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID=\"subs\",NAME=\"%s\",DEFAULT=%s,AUTOSELECT=%s,LANGUAGE=\"%s\",URI=\"%s\"",
			e.Language, isDefault, isDefault, e.LanguageCode, fmt.Sprintf("subtitle/%s/index.m3u8", e.Language),
		))
	}

	temp, err := h.LocalStorage.ReadFileLines(fmt.Sprintf("/%s/%s/master.m3u8", h.cfg.TempFolderPath, transcodeInfo.Pipeline.OutputKey))
	if err != nil {
		return err
	}

	fileLines := []string{}
	fileLines = append(fileLines, temp[:2+len(transcodeInfo.Pipeline.AudioTracks)]...)
	fileLines = append(fileLines, lines...)
	fileLines = append(fileLines, temp[2+len(transcodeInfo.Pipeline.AudioTracks):]...)

	for i, e := range fileLines {
		if i >= 4 && strings.Contains(e, "RESOLUTION") {
			fileLines[i] = e + ",SUBTITLES=\"subs\""
		}
	}

	err = h.LocalStorage.WriteLinesToFile(fileLines, fmt.Sprintf("/%s/%s/master.m3u8", h.cfg.TempFolderPath, transcodeInfo.Pipeline.OutputKey))
	if err != nil {
		return err
	}

	return nil
}

func (h *handlerObj) AddTranscodeQueue(job transcoder.TrInfo) {
	h.videoQueue <- job
}

func (h *handlerObj) AddPreparationQueue(job Job) {
	h.preparationQueue <- job
}

func (h *handlerObj) AddCloudStorageQueue(job transcoder.TrInfo) {
	h.fileQueue <- job
}

func (h *handlerObj) CloudStorageWorker(id int) {
	workerId := "worker[" + strconv.Itoa(id) + "] UPLOADER"
	h.log.Info(workerId, logger.String("action", "[STARTING]"))

	for job := range h.fileQueue {
		h.log.Info("==================== Message is received in Cloud Storage worker ====================")
		h.log.Info(workerId, logger.String("action", "[GET]"), logger.String("message[key]", job.Output))

		h.UploadToCloud(job)
	}
}

func (h *handlerObj) UploadToCloud(req transcoder.TrInfo) {

	updatePipelineStage := req.UpdatePipelineStage
	updatePipelineStage.Stage = h.cfg.Stages.Upload
	updatePipelineStage.Status = h.cfg.Status.Pending
	path := h.LocalStorage.GetUploadPath(req.Pipeline.OutputKey)

	defer func() {
		err := h.LocalStorage.RemoveFromDir(path)
		if err != nil {
			updatePipelineStage.Status = h.cfg.Status.Fail
			updatePipelineStage.FailDescription = "Error while removing content from local storage: " + err.Error()
			updatePipelineStage.ErrorCode = InternalServerError
			err = h.rabbitMQ.PublishPipelineStatus(&updatePipelineStage)
			if err != nil {
				h.log.Error("Error while publishing to rabbit mq.", logger.Error(err))
			}
			h.log.Error("[-] STORAGE: Couldn't delete folder from server", logger.Error(err))
			return
		}
		fmt.Println("[+] STORAGE: Successfully removed from directory")
	}()

	err := h.rabbitMQ.PublishPipelineStatus(&updatePipelineStage)
	if err != nil {
		h.log.Error("Error while publishing to rabbit mq.", logger.Error(err))
	}

	start := time.Now()
	storage, err := storage.NewCloudStorage(h.cfg, &models.CloudStorageConfig{
		Endpoint:  req.Pipeline.CdnUrl,
		AccessKey: req.Pipeline.CdnAccessKey,
		SecretKey: req.Pipeline.CdnSecretKey,
		Type:      req.Pipeline.CdnType,
		Region:    req.Pipeline.CdnRegion,
	}, h.log)
	if err != nil {
		updatePipelineStage.Status = h.cfg.Status.Fail
		updatePipelineStage.FailDescription = "Error while connecting to Cloud: " + err.Error()
		updatePipelineStage.ErrorCode = InvalidRequest
		err = h.rabbitMQ.PublishPipelineStatus(&updatePipelineStage)
		if err != nil {
			h.log.Error("Error while publishing to rabbit mq.", logger.Error(err))
		}
		h.log.Error("[-] Storage Couldn't connect to database", logger.Error(err))
		return
	}

	switch req.Pipeline.CdnType {
	case "minio":
		err = storage.Minio().UploadFilesToCloud(path, req.Pipeline)
	case "s3":
		err = storage.S3().UploadFilesToCloud(path, req.Pipeline)
	default:
		err = fmt.Errorf("invalid cdn storage type")
	}

	if err != nil {
		updatePipelineStage.Status = h.cfg.Status.Fail
		updatePipelineStage.FailDescription = "Error while uploading to Cloud: " + err.Error()
		updatePipelineStage.ErrorCode = InvalidRequest

		err = h.rabbitMQ.PublishPipelineStatus(&updatePipelineStage)
		if err != nil {
			h.log.Error("Error while publishing to rabbit mq.", logger.Error(err))
		}

		h.log.Error("[-] STORAGE Couldn't upload to CDN", logger.Error(err))
		return
	}
	end := time.Since(start)
	updatePipelineStage.Status = h.cfg.Status.Success
	updatePipelineStage.UploadDuration = int(end.Milliseconds())
	err = h.rabbitMQ.PublishPipelineStatus(&updatePipelineStage)
	if err != nil {
		h.log.Error("Error while publishing to rabbit mq.", logger.Error(err))
	}
	h.log.Info("[UPLOADED] SUCCESS", logger.Any("info", path))
}
