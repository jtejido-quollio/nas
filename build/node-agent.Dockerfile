FROM golang:1.23-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/node-agent ./cmd/node-agent

FROM debian:12-slim
RUN set -eux; \
    if [ -f /etc/apt/sources.list.d/debian.sources ]; then \
      sed -i 's/^Components: main$/Components: main contrib non-free non-free-firmware/' /etc/apt/sources.list.d/debian.sources; \
    elif [ -f /etc/apt/sources.list ]; then \
      sed -i 's/ bookworm main/ bookworm main contrib non-free non-free-firmware/g' /etc/apt/sources.list; \
      sed -i 's/ bookworm-updates main/ bookworm-updates main contrib non-free non-free-firmware/g' /etc/apt/sources.list; \
      sed -i 's/ bookworm-security main/ bookworm-security main contrib non-free non-free-firmware/g' /etc/apt/sources.list; \
    fi; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
      util-linux udev gdisk parted smartmontools zfsutils-linux nvme-cli; \
    rm -rf /var/lib/apt/lists/*
COPY --from=build /out/node-agent /usr/local/bin/node-agent
EXPOSE 9808
ENTRYPOINT ["/usr/local/bin/node-agent"]
