FROM golang:1.23-bookworm AS build

WORKDIR /src

RUN apt-get update \
	&& apt-get install -y --no-install-recommends nodejs npm ca-certificates \
	&& rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN npx tailwindcss@3.x.x \
	-c ./frontend/tailwind.config-v3.js \
	-i ./frontend/tailwind-input-v3.css \
	-o ./frontend/assets/style.css \
	--minify

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -tags production -o /out/app ./cmd/server
RUN go install github.com/pressly/goose/v3/cmd/goose@v3.26.0

FROM debian:bookworm-slim AS runtime

RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates wget libsqlite3-0 \
	&& rm -rf /var/lib/apt/lists/*

WORKDIR /data

COPY --from=build /out/app /opt/app/app
COPY --from=build /go/bin/goose /opt/app/goose
COPY --from=build /src/frontend/assets /opt/app/frontend/assets
COPY --from=build /src/sql/migrations /opt/app/sql/migrations
COPY app.prod.toml /opt/app/app.prod.toml
COPY docker-entrypoint.sh /opt/app/docker-entrypoint.sh

RUN chmod +x /opt/app/docker-entrypoint.sh

EXPOSE 5000

ENTRYPOINT ["/opt/app/docker-entrypoint.sh"]
