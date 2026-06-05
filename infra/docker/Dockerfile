FROM golang:1.24-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-X main.Version=$(cat VERSION) -X main.Commit=$(git rev-parse --short HEAD 2>/dev/null || echo unknown) -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o /metering ./cmd/server

FROM alpine:3.21

RUN apk --no-cache add ca-certificates tzdata
COPY --from=builder /metering /usr/local/bin/metering

RUN addgroup -S metering && adduser -S -G metering metering
USER metering

EXPOSE 8080

ENTRYPOINT ["metering"]
