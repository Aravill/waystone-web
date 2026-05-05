# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o waystone-web .

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/
RUN mkdir -p /root/data

COPY --from=builder /app/waystone-web .
COPY static ./static

EXPOSE 8080

CMD ["./waystone-web"]
