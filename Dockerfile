# Build stage
FROM golang:1.27-alpine3.24 AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
ARG APP_VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-X waystone-web/config.Version=${APP_VERSION}" -o waystone-web .

# Runtime stage
FROM alpine:3.24

RUN apk --no-cache add ca-certificates

WORKDIR /root/
RUN mkdir -p /root/data

COPY --from=builder /app/waystone-web .
COPY static ./static

EXPOSE 8080

CMD ["./waystone-web"]
