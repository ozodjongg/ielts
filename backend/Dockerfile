# syntax=docker/dockerfile:1.7
FROM golang:1.23-alpine AS build
ARG SERVICE
WORKDIR /src
RUN apk add --no-cache ca-certificates tzdata git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN test -n "$SERVICE" && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/service ./cmd/${SERVICE}

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata wget && addgroup -S app && adduser -S -G app app
WORKDIR /app
RUN mkdir -p /data/private/listening /data/private/review && chown -R app:app /data
COPY --from=build /out/service /app/service
USER app
ENTRYPOINT ["/app/service"]
