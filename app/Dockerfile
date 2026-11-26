FROM golang:1.27.4-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /server ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates gosu \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 app && useradd --uid 10001 --gid app --no-create-home app
WORKDIR /app
COPY --from=build /server /app/server
COPY resources/meme.jpg /app/resources/meme.jpg
COPY --chmod=755 docker-entrypoint.sh /usr/local/bin/docker-entrypoint
ENV DB_PATH=/data/app.db MEME_PATH=/app/resources/meme.jpg
EXPOSE 8080
ENTRYPOINT ["docker-entrypoint"]
