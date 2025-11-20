CURRENT_DIR=$(shell pwd)

APP=$(shell basename ${CURRENT_DIR})

TAG=latest
ENV_TAG=latest
PROJECT_NAME=ucode
DOCKERFILE=Dockerfile

build-image:
	docker build --rm -t ${REGISTRY}/${PROJECT_NAME}/${APP}:${TAG} . -f ${DOCKERFILE}
	docker tag ${REGISTRY}/${PROJECT_NAME}/${APP}:${TAG} ${REGISTRY}/${PROJECT_NAME}/${APP}:${ENV_TAG}

push-image:
	docker push ${REGISTRY}/${PROJECT_NAME}/${APP}:${TAG}
	docker push ${REGISTRY}/${PROJECT_NAME}/${APP}:${ENV_TAG}

clear-image:
	docker rmi ${REGISTRY}/${PROJECT_NAME}/${APP}:${TAG}
	docker rmi ${REGISTRY}/${PROJECT_NAME}/${APP}:${ENV_TAG}

build:
	go build -o bin/app cmd/main.go
#
#sudo ffmpeg   -vsync passthrough   -hwaccel cuda   -hwaccel_output_format cuda   -extra_hw_frames 5   -threads 3   -i https://minio.salomtv.uz/videosinput/771200945da6907c1d95492165372576   -filter_complex "
#	[0:0]split=3[in0][in1][in2];
#  [in0]scale_npp=426:-1[240p];
#  [in1]scale_npp=640:-1[360p];
#  [in2]scale_npp=854:-1[480p] "   -g 48   -sc_threshold 0   -map [240p] -c:v:0 h264_nvenc -b:v:0 300k -preset fast   -map [360p] -c:v:1 h264_nvenc -b:v:1 500k -preset fast   -map [480p] -c:v:2 h264_nvenc -b:v:2 1.5M -preset fast   -var_stream_map "v:0,name:240p v:1,name:360p v:2,name:480p"   -master_pl_name master.m3u8   -hls_time 3   -hls_list_size 0   -hls_segment_filename //prod-transcode/A10741/%v/fileSequence%d.ts   //prod-transcode/A10741/%v/index.m3u8
#
#sudo ffmpeg -vsync passthrough -hwaccel cuda -hwaccel_output_format cuda -extra_hw_frames 5 -threads 2 -i http://cdn.uzd.udevs.io:9000/uzdigital-temp/05f6e6ad4cdad8fa23a0d67e550066f3 -filter_complex "
#	[0:0]split=1[in0];
#	[in0]scale_npp=854:-1[480p]; " -g 48 -sc_threshold 0 -map [480p] -c:v:0 h264_nvenc -b:v:0 1.5M -var_stream_map v:0,name:480p -master_pl_name master.m3u8 -hls_time 3 -hls_list_size 0 -hls_segment_filename //prod-transcode/test-05f6e6ad4cdad8fa23a0d67e550066f3/%v/fileSequence%d.ts //prod-transcode/test-05f6e6ad4cdad8fa23a0d67e550066f3/%v/index.m3u8
#
#sudo ffmpeg \
#  -vsync passthrough \
#  -hwaccel cuda \
#  -hwaccel_output_format cuda \
#  -extra_hw_frames 5 \
#  -threads 3 \
#  -i https://minio.salomtv.uz/videosinput/5a2c7739bbefb11476c7b8e9e584964f \
#  -filter_complex "
#    [0:0]split=3[in0][in1][in2];
#    [in0]scale_npp=426:-1[240p];
#    [in1]scale_npp=640:-1[360p];
#    [in2]scale_npp=854:-1[480p];
#  " \
#  -g 48 \
#  -sc_threshold 0 \
#  -map [240p] -c:v:0 h264_nvenc -b:v:0 300k \
#  -map [360p] -c:v:1 h264_nvenc -b:v:1 500k \
#  -map [480p] -c:v:2 h264_nvenc -b:v:2 1.5M \
#  -var_stream_map "v:0,name:240p v:1,name:360p v:2,name:480p" \
#  -master_pl_name master.m3u8 \
#  -hls_time 3 \
#  -hls_list_size 0 \
#  -hls_segment_filename //prod-transcode/A00000/%v/fileSequence%d.ts \
#  //prod-transcode/A00000/%v/index.m3u8