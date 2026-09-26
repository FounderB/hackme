# Runtime layout under /opt/hackme: binaries + scripts (worker_loop) + tasks (fuzz gates).
# Set HACKME_WORKING_DIR=/opt/hackme in compose so cwd resolves ./scripts and ./data.
FROM golang:1.26.8-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
ENV GOTOOLCHAIN=local
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.Version=0.1.0-rc17.1" -o /out/hackme .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/coordinator ./cmd/coordinator
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/workerpoh-cpu ./cmd/workerpoh
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/minersign ./cmd/minersign

FROM debian:bookworm-slim
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates curl bash jq python3 \
  && rm -rf /var/lib/apt/lists/*
RUN install -d /opt/hackme
COPY --from=build /out/hackme /opt/hackme/hackme
COPY --from=build /out/coordinator /opt/hackme/coordinator
COPY --from=build /out/workerpoh-cpu /opt/hackme/workerpoh-cpu
COPY --from=build /out/workerpoh-cpu /opt/hackme/workerpoh
COPY --from=build /out/minersign /opt/hackme/minersign
COPY --from=build /src/scripts /opt/hackme/scripts
COPY --from=build /src/tasks /opt/hackme/tasks
RUN ln -sf /opt/hackme/hackme /usr/local/bin/hackme \
  && ln -sf /opt/hackme/coordinator /usr/local/bin/coordinator \
  && ln -sf /opt/hackme/scripts/ops/worker_autostart.sh /opt/hackme/worker_autostart.sh \
  && chmod 755 /opt/hackme/hackme /opt/hackme/coordinator /opt/hackme/workerpoh /opt/hackme/workerpoh-cpu /opt/hackme/minersign \
  && chmod 755 /opt/hackme/scripts/ops/*.sh 2>/dev/null || true
WORKDIR /opt/hackme
EXPOSE 8080 8081
