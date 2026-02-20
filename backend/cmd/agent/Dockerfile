# -- Stage 1: Build agent binary --
FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /agent cmd/agent/main.go

# -- Stage 2: Runtime --
FROM amazonlinux:latest
RUN yum install -y unzip git && update-ca-trust && mkdir -p /tmp/.terraform.d/plugin-cache
COPY --from=builder /agent /tmp/agent
ENTRYPOINT ["/tmp/agent"]
