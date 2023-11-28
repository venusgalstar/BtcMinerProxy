# Use the same base image for building and running
FROM ubuntu:20.04
FROM golang:1.20 as builder

# Set up the build environment
WORKDIR /mnt/app
RUN apt-get update -y && apt-get install -y git
RUN git clone https://github.com/venusgalstar/btcminerproxy.git .
RUN sed -i 's/go 1.21.0/go 1.20/' go.mod  
RUN go mod download
RUN go build .


# Copy the binary into the same base image
FROM golang:1.20

WORKDIR /mnt/app/BtcMinerProxy
COPY btcminerproxy btcminerproxy
COPY config.json config.json

# Expose port and run
EXPOSE 1315
EXPOSE 3333
EXPOSE 3334
CMD ["./btcminerproxy"]
