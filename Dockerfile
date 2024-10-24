FROM golang:1.20-buster AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o kafka-http-server .

FROM debian:bullseye-slim
WORKDIR /app

# Install necessary tools: netcat, telnet, and iputils-ping for ping
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    netcat \
    telnet \
    iputils-ping && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/kafka-http-server .

EXPOSE 8080
CMD ["./kafka-http-server"]
