# Use the same base image for building and running
FROM golang:1.20 as builder

# Set up the build environment
WORKDIR /mnt/app
RUN apt-get update -y && apt-get install -y git
RUN git clone https://github.com/venusgalstar/btcminerproxy.git .
RUN sed -i 's/go 1.21.0/go 1.20/' go.mod  
RUN go mod download
RUN go build -o btcminerproxy
RUN apt-get update && apt-get install redis-server -y 
RUN sed -i 's@bind 127.0.0.1@bind 0.0.0.0@' /etc/redis/redis.conf
RUN sed -i 's@daemonize yes@daemonize no@' /etc/redis/redis.conf

# Copy the binary into the same base image
FROM golang:1.20

WORKDIR /mnt/app
COPY --from=builder /mnt/app/btcminerproxy /mnt/app/btcminerproxy
COPY --from=builder /mnt/app/config.json /mnt/app/config.json

# Expose port and run
EXPOSE 1315
EXPOSE 3333
EXPOSE 6379/tcp
CMD ["./btcminerproxy"]
