FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /go-browser ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates curl
COPY --from=builder /go-browser /usr/local/bin/go-browser
EXPOSE 8906
HEALTHCHECK --interval=15s --timeout=5s --retries=3 CMD curl -sf http://127.0.0.1:8906/health
ENTRYPOINT ["go-browser"]
