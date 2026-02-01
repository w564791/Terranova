tag=$1
GOOS=linux go build .
docker build -t amazonlinux:unzip-$tag .
kind load docker-image  amazonlinux:unzip-$tag
