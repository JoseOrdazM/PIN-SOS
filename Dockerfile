# Stage 1: Build
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o pinsos .

# Stage 2: Runtime
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata sqlite-libs

WORKDIR /app

COPY --from=builder /build/pinsos .
COPY --from=builder /build/static ./static

# Generate simple PNG icons from SVG
RUN apk add --no-cache librsvg && \
    if [ -f static/icon-512.svg ]; then \
      rsvg-convert -w 192 -h 192 static/icon-512.svg -o static/icon-192.png 2>/dev/null || true; \
      rsvg-convert -w 512 -h 512 static/icon-512.svg -o static/icon-512.png 2>/dev/null || true; \
    fi

EXPOSE 8080

VOLUME ["/data"]

CMD ["./pinsos"]
