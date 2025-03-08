# syntax=docker/dockerfile:1
FROM golang:1.24 AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o myserver main.go

# Final image
FROM debian:bookworm-slim
WORKDIR /app
COPY --from=builder /app/myserver /app/myserver

# Expose port 8080
EXPOSE 8080

# Command to run your server
CMD ["/app/myserver"]
