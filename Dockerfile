FROM golang:1.8

WORKDIR /app

COPY bin/openhack-rcon-sidecar-linux-amd64 openhack-rcon-sidecar

CMD ["./openhack-rcon-sidecar"]