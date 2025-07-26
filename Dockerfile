FROM golang:1.19-alpine AS builder

WORKDIR /app
COPY . .

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o shadspace-master ./cmd/master/

FROM alpine:3.16

WORKDIR /app

COPY --from=builder /app/shadspace-master .
COPY configs/master.yaml ./configs/
COPY scripts/wait-for.sh .

RUN apk add --no-cache bash curl

# Create data directory
RUN mkdir -p /app/data

EXPOSE 8080 53798

HEALTHCHECK --interval=30s --timeout=3s \
  CMD curl -f http://localhost:8080/status || exit 1

ENTRYPOINT ["./shadspace-master"]