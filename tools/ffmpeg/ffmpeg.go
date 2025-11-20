package ffmpeg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gitlab.com/transcodeuz/video-transcoder/config"
	"gitlab.com/transcodeuz/video-transcoder/models"
	"gitlab.com/transcodeuz/video-transcoder/pkg/logger"
	"gitlab.com/transcodeuz/video-transcoder/tools/transcoder"
)

// FFmpeg is structure for a tool to convert video
type FFmpeg struct {
	cfg *config.Config
	log logger.Logger
}

// NewFFmpeg returns the pointer for ffmpeg structure
func NewFFmpeg(cfg *config.Config, log logger.Logger) transcoder.Transcoder {
	return &FFmpeg{
		cfg: cfg,
		log: log,
	}
}

// Command is the structure for
type Command struct {
	command []string
}

// Args is the argument to replace ffmpeg command list
type Args struct {
	Index int
	Value string
}

// ReplaceArguments - replaces the given arguments with default arguments in the given index
func (f *Command) ReplaceArguments(args []Args) []string {

	for _, arg := range args {
		f.command[arg.Index] = arg.Value
	}

	return f.command
}

var resizeVideo = Command{
	command: []string{
		"-i",             // 0
		"filename.MOV",   // 1
		"-s",             // 2
		"YxX",            // 3
		"-c:v",           // 4
		"libx264",        // 5
		"-r",             // 6
		"24",             // 7
		"-crf",           // 8
		"28",             // 9
		"-strict",        // 10
		"-2",             // 11
		"-start_number",  // 12
		"0",              // 13
		"-hls_time",      // 14
		"10",             // 15
		"-hls_list_size", // 16
		"0",              // 17
		"-f",             // 18
		"hls",            // 19
		"output2.mp4",    // 20
	},
}

var videoInfo = Command{
	command: []string{
		"-v", "error", "-show_entries", "stream=width,height,codec_type,duration,profile,index:stream_tags=language", "input", "-of", "json",
	},
}

var resizeVideoStory = Command{
	command: []string{
		"-i",             // 0
		"filename.MOV",   // 1
		"-c:v",           // 4
		"libx264",        // 5
		"-r",             // 6
		"24",             // 7
		"-crf",           // 8
		"28",             // 9
		"-strict",        // 10
		"-2",             // 11
		"-start_number",  // 12
		"0",              // 13
		"-hls_time",      // 14
		"10",             // 15
		"-hls_list_size", // 16
		"0",              // 17
		"-f",             // 18
		"hls",            // 19
		"output2.mp4",    // 18
	},
}

var videoDuration = Command{
	command: []string{
		"-i",
		"filename.mp4",
		"-show_entries",
		"format=duration",
		"-v",
		"quiet",
		"-of",
		"csv",
	},
}

var thumb = Command{
	command: []string{
		"-i",
		"filename.mp4",
		"-ss",
		"60",
		"-vf",
		"'thumbnail,scale=640x360'",
		"-frames:v",
		"1",
		"-y",
		"filename.jpg",
	},
}

var videoResolution = Command{
	command: []string{
		"-v",
		"error",
		"-select_streams",
		"v:0",
		"-show_entries",
		"stream=width,height",
		"-of",
		"default=nw=1",
		"input",
	},
}

var ResizeVideoGPU = Command{
	command: []string{
		"-y",                     // 0
		"-vsync",                 // 1
		"passthrough",            // 2
		"-hwaccel",               // 3
		"cuda",                   // 4
		"-hwaccel_output_format", // 5
		"cuda",                   // 6
		"-extra_hw_frames",       // 7
		"5",                      // 8
		"-i",                     // 9
		"input.mp4",              // 10
		"-vf",                    // 11
		"scale_npp=1280:720",     // 12
		"-map",                   // 13
		"0:v:0",                  // 14
		"-map",                   // 15
		"0:a:0?",                 // 16
		"-b:a",                   // 17
		"192k",                   // 18
		"-c:v",                   // 19
		"h264_nvenc",             // 20
		"-b:v",                   // 21
		"8M",                     // 22
		"-g",                     // 23
		"30",                     // 24
		"-hls_time",              // 25
		"10",                     // 26
		"-hls_list_size",         // 27
		"0",                      // 28
		"-hls_flags",             // 29
		"single_file",            // 30
		"-f",                     // 31
		"hls",                    // 32
		"output2.mp4",            // 33
	},
}

func (f *FFmpeg) GetVideoInfo(input string) (*transcoder.VideoInfo, error) {
	f.log.Info("GetVideoInfo", logger.String("input", input))
	var info transcoder.VideoInfo

	commands := videoInfo.ReplaceArguments([]Args{
		{
			Index: 4,
			Value: input,
		},
	})
	f.log.Debug("commands in GetVideoInfo: ", logger.Any("commands: ", commands))

	res, err := exec.Command(f.cfg.FFprobe, commands...).CombinedOutput()
	if err != nil {
		f.log.Debug("commands in GetVideoInfo response: ", logger.Any("response: ", res))
		return nil, err
	}

	err = json.Unmarshal(res, &info)
	if err != nil {
		if len(strings.Split(string(res), `"streams":`)) == 2 {
			resNew := []byte(`{ "streams":` + strings.Split(string(res), `"streams":`)[1])
			errNew := json.Unmarshal(resNew, &info)
			if errNew != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	return &info, nil
}

func (f *FFmpeg) ConvertVideoToMP4(input, outputPath string) error {
	var commands = []string{}
	if f.cfg.UseGPU {
		commands = GpuConvertToMp4.ReplaceArguments([]Args{
			{
				Index: 1,
				Value: input,
			},
			{
				Index: 6,
				Value: outputPath,
			},
		})
	} else {
		commands = CpuConvertToMp4.ReplaceArguments([]Args{
			{
				Index: 1,
				Value: input,
			},
			{
				Index: 2,
				Value: outputPath,
			},
		})
	}
	fmt.Println("ConvertVideoToMP4 command", strings.Join(commands, " "))
	_, err := exec.Command(f.cfg.FFmpeg, commands...).CombinedOutput()
	if err != nil {
		return err
	}

	return nil
}

// ResizeVideo - resizes the video into given measure
func (f *FFmpeg) ResizeVideo(input string, measure string, output string, is_story string) error {
	var commands []string
	f.log.Info(
		"Started Resizing...",
		logger.String("input", input),
		logger.String("measure", measure),
		logger.String("output", output),
	)
	if is_story == "false" {
		commands = resizeVideo.ReplaceArguments([]Args{
			{
				Index: 1,
				Value: input,
			},
			{
				Index: 3,
				Value: measure,
			},
			{
				Index: 20,
				Value: output,
			},
		})
	} else {
		commands = resizeVideoStory.ReplaceArguments([]Args{
			{
				Index: 1,
				Value: input,
			},
			{
				Index: 18,
				Value: output,
			},
		})
	}
	f.log.Debug("commands in ResizeVideo: ", logger.Any("commands: ", commands))

	_, err := exec.Command(f.cfg.FFmpeg, commands...).CombinedOutput()
	if err != nil {
		f.log.Error("Error whiele executing the command", logger.Error(err))
		return err
	}

	f.log.Info("Finished resizing")
	return nil
}

// ResizeVideoGpuMaster Last version
func (f *FFmpeg) ResizeVideoGpuMaster(transcoderInfo transcoder.TrInfo) error {
	var (
		err      error
		commands []string
	)

	f.log.Info("Started Resizing...", logger.String("input", transcoderInfo.Input), logger.String("output", transcoderInfo.Output))

	switch transcoderInfo.UseGPU {
	case true:
		f.log.Info("[+] RESIZING VIDEO GPU", logger.String("STATE", "START"))
		commands = makeDynamicCommand(transcoderInfo)
	case false:
		f.log.Info("[+] RESIZING VIDEO CPU", logger.String("STATE", "START"))
		commands = makeDynamicCommandCpu(transcoderInfo)
	}

	fmt.Println(commands)

	start := time.Now()
	res, err := exec.Command(f.cfg.FFmpeg, commands...).CombinedOutput()

	end := time.Since(start)

	f.log.Info("RESIZE INFO", logger.Any("time", fmt.Sprintf("[%s]", end.String())))
	if err != nil {
		switch transcoderInfo.UseGPU {
		case true:
			f.log.Info("[+] RESIZING VIDEO CPU", logger.String("UPDATE", "START"))
			commands = makeDynamicCommandCpu(transcoderInfo)
			fmt.Println(commands)
			res, err = exec.Command(f.cfg.FFmpeg, commands...).CombinedOutput()
			end = time.Since(start)
			if err == nil {
				f.log.Error("[+] RESIZING VIDEO CPU", logger.String("STATE", "SUCCESS"))
				return nil
			}

			f.log.Error("[-] RESIZING VIDEO GPU", logger.String("STATE", "FAILED"), logger.Error(err))
		case false:
			f.log.Info("[+] RESIZING VIDEO GPU", logger.String("UPDATE", "START"))
			commands = makeDynamicCommand(transcoderInfo)
			fmt.Println(commands)
			res, err = exec.Command(f.cfg.FFmpeg, commands...).CombinedOutput()
			end = time.Since(start)
			if err == nil {
				f.log.Error("[+] RESIZING VIDEO GPU", logger.String("STATE", "SUCCESS"))
				return nil
			}

			f.log.Error("[-] RESIZING VIDEO CPU", logger.String("STATE", "FAILED"), logger.Error(err))
		}
		fmt.Println(string(res))
		return err
	}

	switch transcoderInfo.UseGPU {
	case true:
		f.log.Error("[+] RESIZING VIDEO GPU", logger.String("STATE", "SUCCESS"))
	case false:
		f.log.Error("[+] RESIZING VIDEO CPU", logger.String("STATE", "SUCCESS"))
	}

	return nil
}

// GetThumb - returns the snapshot
func (f *FFmpeg) GetThumb(input string, output string) error {
	commands := thumb.ReplaceArguments([]Args{
		{
			Index: 1,
			Value: input,
		},

		{
			Index: 9,
			Value: output,
		},
	})

	out, err := exec.Command(f.cfg.FFmpeg, commands...).CombinedOutput()
	if err != nil {
		return err
	}

	log.Printf("out:%s \n", out)
	return nil
}

// GetVideo
func (f *FFmpeg) GetVideoWidthHeight(input string) (int, int, error) {
	commands := videoResolution.ReplaceArguments([]Args{
		{
			Index: 8,
			Value: input,
		},
	})

	out, err := exec.Command(f.cfg.FFprobe, commands...).CombinedOutput()
	if err != nil {
		return 0, 0, err
	}

	m := make(map[string]string)
	values := strings.Split(string(out), "\n")
	f.log.Info("Values: ", logger.Any("values", values))

	for _, value := range values {
		measure := strings.Split(value, "=")
		if len(measure) < 2 {
			continue
		}

		m[strings.TrimSpace(measure[0])] = strings.TrimSpace(measure[1])
	}

	width, _ := strconv.Atoi(m["width"])
	height, _ := strconv.Atoi(m["height"])

	return width, height, nil
}

// GetVideoResolution - returns the video resolution
func (f *FFmpeg) GetVideoResolution(input string) (result transcoder.ResolutionFormat, err error) {
	f.log.Info("Received input", logger.String("input", input))

	commands := videoResolution.ReplaceArguments([]Args{
		{
			Index: 8,
			Value: input,
		},
	})
	f.log.Debug("commands in GetVideoResolution: ", logger.Any("commands: ", commands))

	f.log.Info("it is ffprobe", logger.Any("ffprobe: ", f.cfg.FFprobe))
	out, err := exec.Command(f.cfg.FFprobe, commands...).CombinedOutput()
	if err != nil {
		f.log.Error("out error in GetVideoResolution", logger.Any("err: ", err))
		return result, err
	}
	f.log.Debug("out in GetVideoResolution: ", logger.Any("out: ", out))

	result, index := f.getResolutionFormat(string(out))
	f.log.Debug("result in GetVideoResolution: ", logger.Any("result: ", result))
	if index < 0 {
		err = fmt.Errorf("unknown resolution: %s", out)
		f.log.Error("Error while identifying resolution", logger.Error(err))
	}

	return
}

// GetVideoDuration - returns the duration of the given video input
func (f *FFmpeg) GetVideoDuration(input string) int {
	commands := videoDuration.ReplaceArguments([]Args{
		{
			Index: 1,
			Value: input,
		},
	})
	f.log.Debug("commands: ", logger.Any("commands: ", commands))

	f.log.Info("it is ffprobe", logger.Any("ffprobe: ", f.cfg.FFprobe))
	out, err := exec.Command(f.cfg.FFprobe, commands...).CombinedOutput()
	if err != nil {
		f.log.Error("out error in GetVideoDuration", logger.Any("err: ", err))
		return 0
	}
	f.log.Debug("out: ", logger.Any("out: ", out))

	splitStrings := strings.Split(string(out), ",")
	f.log.Debug("splitStrings: ", logger.Any("splitStrings: ", splitStrings))
	output := convertDurationOutputToInt(splitStrings)
	return output
}

func (f *FFmpeg) GetAudioTrack(input, outputPath string, index int) error {
	commands := GetAudioTrack.ReplaceArguments([]Args{
		{Index: 1, Value: input},
		{Index: 3, Value: fmt.Sprintf("0:%d", index)},
		{Index: 7, Value: outputPath},
	})

	fmt.Println("GetAudioTrack command: ", strings.Join(commands, " "))
	_, err := exec.Command(f.cfg.FFmpeg, commands...).CombinedOutput()
	if err != nil {
		fmt.Println(err)
		f.log.Error("out error in GetAudioTrack", logger.Any("err: ", err.Error()))
		return err
	}

	return nil
}

func (f *FFmpeg) CreateAudioStream(track models.AudioTrack, outputPath string) error {
	commands := CreateAudioStreamCPU.ReplaceArguments([]Args{
		{Index: 1, Value: track.InputURL}, // input
		{Index: 11, Value: outputPath},    // output path
	})
	f.log.Info("Creating audio streams" + strings.Join(commands, " "))

	_, err := exec.Command(f.cfg.FFmpeg, commands...).CombinedOutput()
	if err != nil {
		f.log.Error("out error in CreateAudioStream", logger.Any("err: ", err))
		return err
	}

	return nil
}

func (f *FFmpeg) CreateSubtitleStreams(subtitle models.SubtitleRequest, outputkey string) error {
	vttFile := fmt.Sprintf("/%s/%s_%s.vtt", f.cfg.TempInputPath, outputkey, subtitle.LanguageCode)
	parseCommand := SrtToVtt.ReplaceArguments(
		[]Args{
			{Index: 1, Value: subtitle.InputURL},
			{Index: 2, Value: vttFile},
		},
	)
	_, err := exec.Command(f.cfg.FFmpeg, parseCommand...).CombinedOutput()
	if err != nil {
		f.log.Error("out error in CreateSubtitleStreams", logger.Any("err: ", err))
		return err
	}

	commands := CreateSubtitleStreamCommand.ReplaceArguments(
		[]Args{
			{
				Index: 1,
				Value: vttFile,
			},
			{
				Index: 11,
				Value: fmt.Sprintf("/%s/%s/subtitle/%s/index.m3u8", f.cfg.TempFolderPath, outputkey, subtitle.LanguageCode),
			},
			{
				Index: 16,
				Value: fmt.Sprintf("/%s/%s/subtitle/%s/", f.cfg.TempFolderPath, outputkey, subtitle.LanguageCode) + `subtitle_%d.vtt`,
			},
		},
	)
	_, err = exec.Command(f.cfg.FFmpeg, commands...).CombinedOutput()
	if err != nil {
		f.log.Error("out error in CreateSubtitleStreams", logger.Any("err: ", err))
		return err
	}
	fmt.Println("removie func started 5")
	err = os.Remove(vttFile)
	if err != nil {
		f.log.Error("Error while removing temp input vtt file", logger.Any("err: ", err))
		return err
	}

	return nil
}

func (f *FFmpeg) SubtitleFileTOVTTFile(vttFile string, inputURL string) error {
	parseCommand := SrtToVtt.ReplaceArguments(
		[]Args{
			{Index: 1, Value: inputURL},
			{Index: 2, Value: vttFile},
		},
	)
	_, err := exec.Command(f.cfg.FFmpeg, parseCommand...).CombinedOutput()
	if err != nil {
		f.log.Error("out error in CreateSubtitleStreams", logger.Any("err: ", err))
		return err
	}
	return nil
}

func (f *FFmpeg) getResolutionFormat(out string) (result transcoder.ResolutionFormat, index int) {
	m := make(map[string]string)
	values := strings.Split(string(out), "\n")
	f.log.Info("Values: ", logger.Any("values", values))

	for _, value := range values {
		measure := strings.Split(value, "=")
		if len(measure) < 2 {
			continue
		}

		m[strings.TrimSpace(measure[0])] = strings.TrimSpace(measure[1])
	}
	width, _ := strconv.Atoi(m["width"])
	height, _ := strconv.Atoi(m["height"])
	result, index = transcoder.FindResolutionFormat(width, height)

	return
}

func convertDurationOutputToInt(input []string) int {
	outputStr := input[len(input)-1]
	output, err := strconv.ParseFloat(outputStr[0:len(outputStr)-2], 32)

	if err != nil {
		log.Println("Error while converting the string to number", err)
		return 0
	}
	return int(output)
}

func (f *FFmpeg) ExtractSubtitleStream(inputFile string, streamIndex int, Tag string, outputFolder string) (string, error) {
	vttFile := fmt.Sprintf("/%s/%s.vtt", outputFolder, Tag)
	cmd := exec.Command("ffmpeg", "-i", inputFile, "-map", fmt.Sprintf("0:%d", streamIndex), vttFile)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("ffmpeg error: %v, stderr: %s", err, stderr.String())
	}
	return vttFile, nil
}

func (f *FFmpeg) GetSubtitleStreams(inputFile string) ([]models.SubtitleStream, error) {
	var (
		streams []models.SubtitleStream
		out     bytes.Buffer
	)
	cmd := exec.Command("ffprobe", "-v", "error", "-select_streams", "s", "-show_entries", "stream=index:tags=title:stream_tags=language", "-of", "csv=p=0", inputFile)
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(out.String(), "\n")
	for _, line := range lines {
		if line != "" {
			fmt.Println(line)
			parts := strings.Split(line, ",")
			if len(parts) >= 2 {
				indexStr := parts[0]
				new_tag := parts[1]
				if len(parts) >= 3 {
					new_tag += "_" + strings.Split(parts[2], " ")[0]
				}
				index, err := strconv.Atoi(indexStr)
				if err != nil {
					f.log.Error("failed to convert index to integer: %v", logger.Error(err))
					continue
				}
				stream := models.SubtitleStream{
					Index: index,
					Tag:   new_tag,
				}
				streams = append(streams, stream)
			}
		}
	}

	return streams, nil
}

func (f *FFmpeg) GetSubtitleName(inputFile string) (map[int]string, error) {
	var (
		streams map[int]string
		out     bytes.Buffer
	)
	streams = make(map[int]string)
	cmd := exec.Command("ffprobe", "-v", "error", "-select_streams", "s", "-show_entries", "stream=index:tags=title:stream_tags=language", "-of", "csv=p=0", inputFile)
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(out.String(), "\n")
	for _, line := range lines {
		if line != "" {
			fmt.Println(line)
			parts := strings.Split(line, ",")
			if len(parts) >= 1 {
				indexStr := parts[0]
				lang := ""
				if len(parts) >= 2 {
					lang = parts[1]
				}
				if len(parts) >= 3 {
					if parts[2] != "" {
						text := strings.ReplaceAll(parts[2], " | ", "_")
						text = strings.ReplaceAll(text, " ", "_")
						re := regexp.MustCompile(`[^a-zA-Zа-яА-Я0-9_]`)
						lang += "_" + re.ReplaceAllString(text, "")
					}
				}
				// Extract index as integer
				index, err := strconv.Atoi(indexStr)
				if err != nil {
					f.log.Error("failed to convert index to integer: %v", logger.Error(err))
					continue
				}

				if lang == "" {
					continue
				}

				// Create SubtitleStream instance
				streams[index] = lang
			}
		}
	}

	return streams, nil
}

func (f *FFmpeg) GetAudioStreamIndexes(inputFile string) ([]models.AudioStream, error) {
	var streams []models.AudioStream
	cmd := exec.Command("ffprobe", "-v", "error", "-select_streams", "a", "-show_entries", "stream=index:tags=title:stream_tags=language", "-of", "csv=p=0", inputFile)
	fmt.Println("Command get streams", "ffprobe", "-v", "error", "-select_streams", "a", "-show_entries", "stream=index:tags=title:stream_tags=language", "-of", "csv=p=0", inputFile)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line != "" {
			fmt.Println(line)
			parts := strings.Split(line, ",")
			if len(parts) >= 2 {
				indexStr := parts[0]
				new_tag := parts[1]
				if len(parts) >= 3 {
					new_tag += "_" + strings.Split(parts[2], " ")[0]
				}
				// Extract index as integer
				index, err := strconv.Atoi(indexStr)
				if err != nil {
					f.log.Error("failed to convert index to integer: %v", logger.Error(err))
					continue
				}

				// Create SubtitleStream instance
				stream := models.AudioStream{
					Index: index,
					Lang:  new_tag,
				}

				// Append to streams slice
				streams = append(streams, stream)
			} else if len(parts) == 1 {
				indexStr := parts[0]
				new_tag := "rus"

				index, err := strconv.Atoi(indexStr)
				if err != nil {
					f.log.Error("failed to convert index to integer: %v", logger.Error(err))
					continue
				}

				// Create SubtitleStream instance
				stream := models.AudioStream{
					Index: index,
					Lang:  new_tag,
				}

				// Append to streams slice
				streams = append(streams, stream)
			}
		}
	}

	f.log.Info("Successfully extracted audio streams")
	return streams, nil
}

func (f *FFmpeg) GetAudioStreamName(inputFile string) (map[int]string, error) {
	streams := make(map[int]string)
	cmd := exec.Command("ffprobe", "-v", "error", "-select_streams", "a", "-show_entries", "stream=index:tags=title:stream_tags=language", "-of", "csv=p=0", inputFile)
	fmt.Println("Command get streams", "ffprobe", "-v", "error", "-select_streams", "a", "-show_entries", "stream=index:tags=title:stream_tags=language", "-of", "csv=p=0", inputFile)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line != "" {
			fmt.Println(line)
			parts := strings.Split(line, ",")
			if len(parts) >= 1 {
				indexStr := parts[0]
				lang := ""
				if len(parts) >= 2 {
					lang = parts[1]
				}
				if len(parts) >= 3 {
					if parts[2] != "" {
						text := strings.ReplaceAll(parts[2], " | ", "_")
						text = strings.ReplaceAll(text, " ", "_")
						re := regexp.MustCompile(`[^a-zA-Zа-яА-Я0-9_]`)
						lang += "_" + re.ReplaceAllString(text, "")
					}
				}
				// Extract index as integer
				index, err := strconv.Atoi(indexStr)
				if err != nil {
					f.log.Error("failed to convert index to integer: %v", logger.Error(err))
					continue
				}

				if lang == "" {
					lang = "rus"
				}

				// Create SubtitleStream instance
				streams[index] = lang
			}
		}
	}

	f.log.Info("Successfully extracted audio streams")
	return streams, nil
}
