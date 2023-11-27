# Use the same base image for building and running
FROM golang:1.20 as builder

# Set up the build environment
WORKDIR /mnt/app
RUN apt-get update -y && apt-get install -y git
RUN git clone https://github.com/venusgalstar/btcminerproxy.git .
RUN sed -i 's/go 1.21.0/go 1.20/' go.mod  
RUN go mod download
RUN go build -o btcminerproxy
RUN apt-get update && apt-get install -y redis-server


# Copy the binary into the same base image
FROM golang:1.20

WORKDIR /mnt/app
COPY --from=builder /mnt/app/btcminerproxy /mnt/app/btcminerproxy
COPY --from=builder /mnt/app/config.json /mnt/app/config.json
COPY --from=builder /mnt/app/start.sh /mnt/app/start.sh 

# Expose port and run
EXPOSE 1315
EXPOSE 3333
CMD start.sh
