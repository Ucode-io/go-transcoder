FROM golang:1.20-alpine AS builder

ENV GO111MODULE=on
ENV GOOS=linux
ENV CGO_ENABLED=0

RUN apk add --no-cache git make

WORKDIR /app

COPY . .

RUN go mod vendor && make build

FROM docker pull ghcr.io/ucode-io/go-transcoder-image:cpu
RUN apt-get update && apt-get install -y ffmpeg && rm -rf /var/lib/apt/lists/*


COPY --from=builder /app/bin/app .
COPY --from=builder /app/.env .

ENTRYPOINT ["./app"]