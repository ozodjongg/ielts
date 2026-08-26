#!/bin/sh
set -eu

mkdir -p /data/private/listening /data/private/review
chown -R app:app /data

exec su-exec app /app/backend
