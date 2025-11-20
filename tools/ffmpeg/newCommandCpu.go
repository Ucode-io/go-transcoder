package ffmpeg

import (
	"fmt"
	"strconv"

	"gitlab.com/transcodeuz/video-transcoder/tools/transcoder"
)

var cpuBaseCommand = Command{
	command: []string{
		"-y", "-i", "input", "-preset", "slow", "-sc_threshold", "0",
		//0    1      2
	},
}

var cpuStreamVideo = Command{
	command: []string{
		"-map", "0:v:0", "-s:v:0", "640x360", "-c:v:0", "libx264", "-b:v:0", "145k",
		// 0		1			2			3			4		5				6		7
	},
}

var CpuConvertToMp4 = Command{
	command: []string{
		"-i", "input-file", "output_file",
		// 	  0 	    1             2
	},
}

var CreateAudioStreamCPU = Command{
	command: []string{
		"-i", "audio.mp3", "-c:a", "aac", "-b:a", "128k", "-vn", "-hls_time", "3", "-hls_list_size", "0", "output_folder/index.m3u8",
		//0        1          2      3       4		5 		6		7 			8 		9				10    	11
	},
}

func makeCpuStreamVideo(format transcoder.ResolutionFormat) []string {
	mapVideos := make([]string, 0, 50)

	for i := 0; i <= format.Priority; i++ {
		mapVideos = append(mapVideos, cpuStreamVideo.ReplaceArguments([]Args{
			{
				Index: 1,
				Value: fmt.Sprintf("0:%d", format.Index),
			},
			{
				Index: 2,
				Value: "-s:v:" + strconv.Itoa(i),
			},
			{
				Index: 3,
				Value: transcoder.ResolutionsPriorityUpdate[i].Measure,
			},
			{
				Index: 4,
				Value: "-c:v:" + strconv.Itoa(i),
			},
			{
				Index: 6,
				Value: "-b:v:" + strconv.Itoa(i),
			},
			{
				Index: 7,
				Value: transcoder.ResolutionsPriorityUpdate[i].VideoBitRate,
			},
		})...)
	}

	return mapVideos
}

func MakeCpuMapping(streams []transcoder.Stream) []string {
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

func makeDynamicCommandCpu(info transcoder.TrInfo) []string {
	command := make([]string, 0, 50)
	command = append(command, cpuBaseCommand.ReplaceArguments([]Args{
		{
			Index: 2,
			Value: info.Input,
		},
	})...)

	command = append(command, makeCpuStreamVideo(info.Resolution)...)
	command = append(command, makeStreamMap(info.Resolution)...)
	command = append(command, hlsMaster.ReplaceArguments([]Args{
		{
			Index: 7,
			Value: info.Output + "/%v/fileSequence%d.ts",
		},
		{
			Index: 8,
			Value: info.Output + "/%v/index.m3u8",
		},
	})...)

	return command
} // there will be 3 degree 0 -- for gpu, 1 -- for gpu with  copy original, 2 -- for cpu
