package ffmpeg

import (
	"fmt"
	"strconv"
	"strings"

	"gitlab.com/transcodeuz/video-transcoder/tools/transcoder"
)

var baseCommand = Command{
	command: []string{
		"-y", "-vsync", "passthrough", "-hwaccel", "cuda", "-hwaccel_output_format", "cuda", "-extra_hw_frames", "5", "-threads", "10", "-i", "input",
		//0 	1			2				3		 4			      5				    6			7			  8		 9	       10     11   12
	},
}

var scaleForm = Command{
	command: []string{
		";", "[in0]", "scale_npp=320:-1", "[240p]",
		//0		1		    2				 3
	},
}

var GpuConvertToMp4 = Command{
	command: []string{
		"-i", "input-file", "-c:v", "h264_nvenc", "-preset", "fast", "output_file",
		// 	  0 	    1          2      	  3 			4 		 5 			6
	},
}

func scaleNpp(format transcoder.ResolutionFormat) []string {
	scale := make([]string, 0, 50)
	for i := 0; i <= format.Priority; i++ {
		xAndY := strings.Split(transcoder.ResolutionsPriorityUpdate[i].Measure, "x")
		gpuMeasure := fmt.Sprintf("scale_npp=%s:%s", xAndY[0], "-1") // xAndY[1]
		scale = append(scale, scaleForm.ReplaceArguments([]Args{
			{
				Index: 1,
				Value: "[in" + strconv.Itoa(i) + "]",
			},
			{
				Index: 2,
				Value: gpuMeasure,
			},
			{
				Index: 3,
				Value: "[" + transcoder.ResolutionsPriorityUpdate[i].Resolution + "]",
			},
		})...)
	}
	return scale
}

func makeFilterComplex(format transcoder.ResolutionFormat) []string {
	count := format.Priority + 1
	command := make([]string, 0, 50) // with additional components

	command = append(command, "-filter_complex", fmt.Sprintf("[0:%d]split=%d", format.Index, count))

	for i := 0; i <= format.Priority; i++ {
		command = append(command, "[in"+strconv.Itoa(i)+"]")
	}

	command = append(command, scaleNpp(format)...)
	result := strings.Join(command[1:], "")

	fmt.Println("command Filter Complex", command[0], result)

	return []string{command[0], result}
}

var mapFilter = Command{
	command: []string{
		"-map", "[240p]", "-c:v:0", "h264_nvenc", "-b:v:0", "145k",
		// 0		1			2			3		4		5				6		7
	},
}

func makeMappingVideo(format transcoder.ResolutionFormat) []string {
	mapVideos := make([]string, 0, 50)

	for i := 0; i <= format.Priority; i++ {
		mapVideos = append(mapVideos, mapFilter.ReplaceArguments([]Args{
			{
				Index: 1,
				Value: "[" + transcoder.ResolutionsPriorityUpdate[i].Resolution + "]", // todo check
			},
			{
				Index: 2,
				Value: "-c:v:" + strconv.Itoa(i),
			},
			{
				Index: 4,
				Value: "-b:v:" + strconv.Itoa(i),
			},
			{
				Index: 5,
				Value: transcoder.ResolutionsPriorityUpdate[i].VideoBitRate,
			},
		})...)
	}

	return mapVideos
}

var mapAudioFilter = Command{
	command: []string{
		"-map", "0:a:0", "-c:a", "aac", "-b:a", "128k", "-ac", "2",
	},
}

func MakeMappingAudio(streams []transcoder.Stream) []string {
	mapAudio := make([]string, 0, 50)

	n := 0
	for _, stream := range streams {
		if stream.CodecType == "audio" {
			mapAudio = append(mapAudio, mapAudioFilter.ReplaceArguments([]Args{
				{
					Index: 1,
					Value: "0:" + strconv.Itoa(stream.Index),
				},
				{
					Index: 2,
					Value: "-c:a:" + strconv.Itoa(n),
				},
			})...)
			n++
		}
	}

	fmt.Println("makeMappingAudio", mapAudio)
	return mapAudio
}

func streamVar(index int) string {
	return fmt.Sprintf("v:%d,name:%s", index, transcoder.ResolutionsPriorityUpdate[index].Resolution)
}

func makeStreamMap(format transcoder.ResolutionFormat) []string {
	stream := make([]string, 0, 100)
	stream = append(stream, "-var_stream_map")

	for i := 0; i <= format.Priority; i++ {
		stream = append(stream, streamVar(i))
	}

	result := strings.Join(stream[1:], " ")
	return []string{stream[0], result}
}

// var copyResolution = Command{
// 	command: []string{
// 		"-map", "0:v:0", "-c:v:4", "copy", "-b:v:4", "8M",
// 	},
// }

// func makeCopyResolution(format transcoder.ResolutionFormat) []string {
// 	return copyResolution.ReplaceArguments([]Args{
// 		{
// 			Index: 2,
// 			Value: "-c:v:" + strconv.Itoa(format.Priority),
// 		},
// 		{
// 			Index: 4,
// 			Value: "-b:v:" + strconv.Itoa(format.Priority),
// 		},
// 		{
// 			Index: 5,
// 			Value: format.VideoBitRate,
// 		},
// 	})
// }

var hlsMaster = Command{
	command: []string{
		"-master_pl_name", "master.m3u8", "-hls_time", "3",
		"-hls_list_size", "0", "-hls_segment_filename",
		"%v/fileSequence%d.ts", "%v/index.m3u8",
	},
}

var presetOptions = Command{
	command: []string{
		"-g", "48", "-sc_threshold", "0",
	},
}

var GetAudioTrack = Command{
	command: []string{
		"-i", "input.mp4", "-map", "0:a:0", "-acodec", "libmp3lame", "-y", "output.acc",
	},
}

var SrtToVtt = Command{
	command: []string{
		"-i", "input.srt", "output.vtt",
	},
}

/*
1. input.vtt
11. output/index.m3u8
16. output/subtitle_%d.vtt
*/
var CreateSubtitleStreamCommand = Command{
	command: []string{
		"-i", "input.vtt", "-map", "0:s:0", "-c", "copy", "-f", "segment", "-segment_time", "10", "-segment_list", "output/index.m3u8", "-segment_format", "webvtt", "-segment_list_flags", "+live", "output/subtitle_%d.vtt",
	},
}

func makeDynamicCommand(transcoderInfo transcoder.TrInfo) []string {
	command := make([]string, 0, 100)
	threadCount := "3"
	if transcoderInfo.Duration < 400 {
		threadCount = "2"
	}

	command = append(command, baseCommand.ReplaceArguments([]Args{
		{
			Index: 12,
			Value: transcoderInfo.Input,
		},
		{
			Index: 10,
			Value: threadCount,
		},
	})...)

	command = append(command, makeFilterComplex(transcoderInfo.Resolution)...)
	command = append(command, presetOptions.ReplaceArguments([]Args{})...)
	command = append(command, makeMappingVideo(transcoderInfo.Resolution)...)
	command = append(command, makeStreamMap(transcoderInfo.Resolution)...)
	command = append(command, hlsMaster.ReplaceArguments([]Args{
		{
			Index: 7,
			Value: transcoderInfo.Output + "/%v/fileSequence%d.ts",
		},
		{
			Index: 8,
			Value: transcoderInfo.Output + "/%v/index.m3u8",
		},
	})...)

	return command
} // there will be 3 degree 0 -- for gpu, 1 -- for gpu with  copy original, 2 -- for cpu
