FROM node:20-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN npm install
COPY web/ ./
RUN npm run build

FROM golang:1.24-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=0 go build -o /out/nas-api ./cmd/nas-api

FROM debian:12-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
  && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/nas-api /usr/local/bin/nas-api
COPY --from=web /web/dist /usr/share/nas-ui
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/nas-api"]
