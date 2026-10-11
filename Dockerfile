FROM golang:1.27.2-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /openmcdb ./cmd/openmcdb

FROM debian:bookworm-slim
EXPOSE 8000
VOLUME /data
RUN apt-get update && apt-get install -y --no-install-recommends wget && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /data && chown nobody:nogroup /data
COPY --from=build /openmcdb /openmcdb
WORKDIR /data
USER nobody
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s CMD wget -qO- http://127.0.0.1:8000/v1/health | grep -q '"ok":true'
ENTRYPOINT ["/openmcdb"]
