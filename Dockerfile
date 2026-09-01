# syntax=docker/dockerfile:1.7
FROM golang:1.23-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates tzdata git
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/backend ./cmd/backend \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/compact-migrate ./cmd/compact-migrate \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/bootstrap-admin ./cmd/bootstrap-admin \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/vocab-import ./cmd/vocab-import \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/reset-password ./cmd/reset-password \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/seed-demo ./cmd/seed-demo

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata curl tini su-exec postgresql-client \
 && addgroup -S app \
 && adduser -S -G app app
WORKDIR /app
COPY --from=build /out/backend /app/backend
COPY --from=build /out/compact-migrate /app/compact-migrate
COPY --from=build /out/bootstrap-admin /app/bootstrap-admin
COPY --from=build /out/vocab-import /app/vocab-import
COPY --from=build /out/reset-password /app/reset-password
COPY --from=build /out/seed-demo /app/seed-demo
COPY backend/migrations /app/backend-migrations
COPY data/english-bank /app/data/english-bank
COPY data/sat-math-bank /app/data/sat-math-bank
COPY data/vocabulary /app/data/vocabulary
COPY data/placement /app/data/placement
COPY deploy/docker-entrypoint.sh /app/docker-entrypoint.sh
RUN mkdir -p /data/private/listening /data/private/review \
 && chown -R app:app /app /data \
 && chmod +x /app/backend /app/compact-migrate /app/bootstrap-admin /app/vocab-import /app/reset-password /app/seed-demo /app/docker-entrypoint.sh
ENV MIGRATIONS_DIR=/app/backend-migrations \
    ENGLISH_BANK_DIR=/app/data/english-bank \
    SAT_BANK_DIR=/app/data/sat-math-bank \
    PLACEMENT_PAPER_PATH=/app/data/placement/placement-test-paper.docx \
    PLACEMENT_PAPER_MANIFEST_PATH=/app/data/placement/paper-v1.json \
    LISTENING_STORAGE_DIR=/data/private/listening \
    REVIEW_STORAGE_DIR=/data/private/review \
    AUTO_MIGRATE=false \
    VOCAB_AUTO_IMPORT=true \
    VOCAB_DATASET_VERSION=stage1-clean-v1 \
    VOCAB_WORDS_FILE=/app/data/vocabulary/generated/stage1_import_ready.csv \
    DB_MAX_CONNS=4 \
    DB_MIN_CONNS=0
EXPOSE 8080
ENTRYPOINT ["/sbin/tini","--","/app/docker-entrypoint.sh"]
