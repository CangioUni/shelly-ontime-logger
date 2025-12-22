FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o shelly-logger

FROM alpine:3.18

WORKDIR /app

COPY --from=builder /app/shelly-logger .
# Copy example config if desired, but user should mount it
COPY config.yaml .

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates tzdata

CMD ["./shelly-logger"]
