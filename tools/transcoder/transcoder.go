package transcoder

import (
	"errors"
	"strconv"
	"strings"

	"gitlab.com/transcodeuz/video-transcoder/models"
)

const (

	// Resolution4K - 3840x2160
	Resolution4K = "4k"
	// ResolutionFullHD - 1920x1080
	ResolutionFullHD = "1080p"
	// ResolutionHD - 1280x720
	ResolutionHD = "720p"
	// ResolutionSD - 854x480
	ResolutionSD = "480p"
	// ResolutionLD - 640x360
	ResolutionLD = "360p"
	// ResolutionVLD - 426x240
	ResolutionVLD = "240p"
)

// ResolutionFormat is the format for resolutions
type ResolutionFormat struct {
	Resolution   string
	Measure      string
	VideoBitRate string
	Priority     int
	Index        int
}

// Transcoder is methods that transcoder must have
type Transcoder interface {
	ResizeVideoGpuMaster(transcoderInfo TrInfo) error
	ResizeVideo(input string, measure string, output string, story string) error
	GetVideoResolution(input string) (ResolutionFormat, error)
	GetVideoWidthHeight(input string) (int, int, error)
	GetThumb(input string, output string) error
	GetVideoDuration(input string) int
	GetVideoInfo(input string) (*VideoInfo, error)
	ConvertVideoToMP4(input, outputPath string) error
	GetAudioTrack(input, outputPath string, index int) error
	CreateAudioStream(track models.AudioTrack, outputPath string) error
	CreateSubtitleStreams(subtitle models.SubtitleRequest, outputkey string) error
	GetSubtitleStreams(inputFile string) ([]models.SubtitleStream, error)
	GetSubtitleName(inputFile string) (map[int]string, error)
	GetAudioStreamIndexes(inputFile string) ([]models.AudioStream, error)
	GetAudioStreamName(inputFile string) (map[int]string, error)
	ExtractSubtitleStream(inputFile string, streamIndex int, outputkey string, outputFile string) (string, error)
	SubtitleFileTOVTTFile(vttFile string, inputURL string) error
}

type TrInfo struct {
	FileName            string
	Input               string
	Output              string
	UploadPath          string
	UseGPU              bool
	VideoInfo           VideoInfo
	Duration            float64
	Resolution          ResolutionFormat
	Pipeline            *models.Pipeline
	UpdatePipelineStage models.UpdatePipelineStage
}

type VideoInfo struct {
	Streams []Stream `json:"streams,omitempty"`
}

type Stream struct {
	Index     int    `json:"index,omitempty"`
	Profile   string `json:"profile,omitempty"`
	CodecType string `json:"codec_type,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Duration  string `json:"duration,omitempty"`
	Tags      struct {
		Language string `json:"language,omitempty"`
	} `json:"tags,omitempty"`
}

func (v VideoInfo) GetStreamWithHighResolution() Stream {
	var (
		res Stream = Stream{
			Index:  -1,
			Width:  -1,
			Height: -1,
		}
	)

	for _, stream := range v.Streams {
		if stream.CodecType == "video" {
			if stream.Width > res.Width {
				res = stream
			} else if stream.Width == res.Width {
				if stream.Height > res.Height {
					res = stream
				}
			}
		}
	}
	return res
}

// ResolutionsPriority - is the set of ordered priorities
var ResolutionsPriority = []ResolutionFormat{
	{
		Resolution:   Resolution4K,
		Measure:      "3840x2160",
		VideoBitRate: "6M",
		Priority:     0,
	},
	{
		Resolution:   ResolutionFullHD,
		Measure:      "1920x1080",
		VideoBitRate: "4M",
		Priority:     1,
	},
	{
		Resolution:   ResolutionHD,
		Measure:      "1280x720",
		VideoBitRate: "3M",
		Priority:     2,
	},
	{
		Resolution:   ResolutionSD,
		Measure:      "854x480",
		VideoBitRate: "1.5M",
		Priority:     3,
	},
	{
		Resolution:   ResolutionLD,
		Measure:      "640x360",
		VideoBitRate: "500k",
		Priority:     4,
	},
	{
		Resolution:   ResolutionVLD,
		Measure:      "426x240",
		VideoBitRate: "300k",
		Priority:     5,
	},
}

var ResolutionsPriorityUpdate = []ResolutionFormat{
	{
		Resolution:   ResolutionVLD,
		Measure:      "426x240",
		VideoBitRate: "300k",
		Priority:     0,
	},
	{
		Resolution:   ResolutionLD,
		Measure:      "640x360",
		VideoBitRate: "500k",
		Priority:     1,
	},
	{
		Resolution:   ResolutionSD,
		Measure:      "854x480",
		VideoBitRate: "1.5M",
		Priority:     2,
	},
	{
		Resolution:   ResolutionHD,
		Measure:      "1280x720",
		VideoBitRate: "3M",
		Priority:     3,
	},
	{
		Resolution:   ResolutionFullHD,
		Measure:      "1920x1080",
		VideoBitRate: "4M",
		Priority:     4,
	},
	{
		Resolution:   Resolution4K,
		Measure:      "3840x2160",
		VideoBitRate: "6M",
		Priority:     5,
	},
}

// GetMeasure - returns the width and height of the wxb
func (r *ResolutionFormat) GetMeasure() (int, int) {
	list := strings.Split(r.Measure, "x")
	if len(list) > 1 {
		width, _ := strconv.Atoi(list[0])
		height, _ := strconv.Atoi(list[1])
		return width, height
	}
	return -1, -1
}

func getMax() int {
	var width int
	for _, res := range ResolutionsPriorityUpdate {
		w, _ := res.GetMeasure()
		if width < w {
			width = w
		}
	}
	return width
}

// FindResolutionFormat returns the ResolutionFormat structure
func FindResolutionFormat(width, height int) (ResolutionFormat, int) {
	// checkpoint -> given width should be less than max of we have
	// Example: width must be less than 4k
	if width > getMax() {
		return ResolutionsPriorityUpdate[5], 0
	}

	var index int = -1

	for i, v := range ResolutionsPriorityUpdate {
		w, _ := v.GetMeasure()
		if width == w {
			return v, i
		} else if width > w {
			index = i
		}
	}

	if index == -1 {
		return ResolutionFormat{}, -1
	} else {
		return ResolutionsPriorityUpdate[index], index
	}
}

func GetResolutionFormat(info VideoInfo) (ResolutionFormat, error) {
	var (
		stream                   Stream
		errCantGetHighResolution error = errors.New("cant get highest resolution")
		res                      ResolutionFormat
	)
	stream = info.GetStreamWithHighResolution()

	res, index := FindResolutionFormat(stream.Width, stream.Height)
	res.Index = stream.Index

	if index < 0 {
		return ResolutionFormat{}, errCantGetHighResolution
	}
	return res, nil
}
