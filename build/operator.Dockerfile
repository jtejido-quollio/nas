FROM golang:1.23-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/operator ./cmd/operator

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/operator /operator
USER 65532:65532
ENTRYPOINT ["/operator"]
