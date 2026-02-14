# -- Stage 1: Build backend --
FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /iac-platform main.go

# -- Stage 2: Runtime --
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /iac-platform .
EXPOSE 8080 8090
ENTRYPOINT ["./iac-platform"]
