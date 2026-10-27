# syntax=docker/dockerfile:1
FROM golang:1.26.5-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY pkg/ ./pkg/
ENV CGO_ENABLED=0
RUN go build -trimpath -o /out/aries ./cmd/aries \
    && go build -trimpath -o /out/aries-ssh-client ./cmd/aries-ssh-client \
    && go build -trimpath -o /out/aries-model-replay ./cmd/aries-model-replay \
    && go build -trimpath -o /out/aries-perf ./cmd/aries-perf

FROM debian:bookworm-slim
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
WORKDIR /app
COPY --from=build --chmod=0555 /out/ /app/bin/
COPY profiles/cloud/ /app/profiles/cloud/
COPY configs/versions.json /app/configs/versions.json
ENV PATH="/app/bin:${PATH}"
# External mode uses the host Docker API; only this trusted controller receives
# the socket. Native OpenClaw runs in its own dynamically created container.
ENTRYPOINT ["/app/bin/aries"]
CMD ["cloud", "profiles/cloud/openclaw-e2b-external-record.json"]
