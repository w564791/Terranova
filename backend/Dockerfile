# -- Stage 1: Build backend --
FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG COMMIT_HASH=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 go build -ldflags="-s -w \
    -X iac-platform/internal/version.CommitHash=${COMMIT_HASH} \
    -X iac-platform/internal/version.BuildTime=${BUILD_TIME}" \
    -o /iac-platform main.go

# -- Stage 2: Runtime --
FROM amazonlinux:2023
RUN dnf install -y ca-certificates tzdata git unzip shadow-utils && dnf clean all
ENV TZ=Asia/Singapore
WORKDIR /app
COPY --from=builder /iac-platform .
EXPOSE 8080 8090
ENTRYPOINT ["./iac-platform"]
