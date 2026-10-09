FROM golang:1.26.9-bookworm@sha256:d9c68c2c51161e12fd77e4c6320687c9cd86e1af1e3ad6e6cd63ff970641453c AS build

WORKDIR /src

RUN apt-get update \
	&& apt-get install -y --no-install-recommends nodejs npm ca-certificates \
	&& rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN npx --yes tailwindcss@3.4.19 \
	-c ./frontend/tailwind.config-v3.js \
	-i ./frontend/tailwind-input-v3.css \
	-o ./frontend/assets/style.css \
	--minify

ARG TARGETOS=linux
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -tags production -o /out/app ./cmd/server
RUN go install github.com/pressly/goose/v3/cmd/goose@v3.26.0

FROM debian:bookworm-slim@sha256:7c7b2c966bc9ee8cedfeef67e0e279108992c77681fa595db4a9d65c06ccc587 AS runtime

RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates wget libsqlite3-0 sqlite3 \
	&& rm -rf /var/lib/apt/lists/*

WORKDIR /data

COPY --from=build /out/app /opt/app/app
COPY --from=build /go/bin/goose /opt/app/goose
COPY --from=build /src/frontend/assets /opt/app/frontend/assets
COPY --from=build /src/sql/migrations /opt/app/sql/migrations
COPY app.prod.toml /opt/app/app.prod.toml
COPY docker-entrypoint.sh /opt/app/docker-entrypoint.sh

RUN chmod +x /opt/app/docker-entrypoint.sh \
    && mkdir -p /data \
    && chown 10001:10001 /data

USER 10001:10001

EXPOSE 5000

ENTRYPOINT ["/opt/app/docker-entrypoint.sh"]
