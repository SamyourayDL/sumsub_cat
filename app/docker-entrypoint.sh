#!/bin/sh
set -eu
# A newly mounted Fly Volume belongs to root. Only initialize its root directory;
# never recursively change user data. The HTTP process always runs unprivileged.
if [ "$(id -u)" = 0 ]; then
    mkdir -p /data
    chown app:app /data
    chmod 700 /data
    exec gosu app:app /app/server
fi
exec /app/server
