# The model bundle is private host data, never part of the build context copies.
FROM docker.io/library/golang:1.27.3-trixie@sha256:af1bfc2614ebb1dbd31af514b78740fd678850e76498762b425ecee4c5c6ea80 AS build
WORKDIR /src
ARG TARGETARCH
RUN test "$TARGETARCH" = arm64 \
    && apt-get update && apt-get install -y --no-install-recommends jq \
    && rm -rf /var/lib/apt/lists/*
COPY go.mod go.sum ./
RUN go mod download
COPY apps/queryembed/ ./apps/queryembed/
COPY internal/embedding/ ./internal/embedding/
COPY scripts/queryembed-native.sh ./scripts/queryembed-native.sh
# The installer verifies both downloaded archives against native.lock before extraction.
RUN bash scripts/queryembed-native.sh linux-arm64
COPY deploy/images/healthcheck/ ./deploy/images/healthcheck/
RUN CGO_ENABLED=1 CGO_LDFLAGS=-L/src/work/native/linux-arm64 \
      go build -tags ort -trimpath -ldflags='-s -w' -o /out/queryembed ./apps/queryembed/cmd/queryembed \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/healthcheck ./deploy/images/healthcheck

FROM gcr.io/distroless/cc-debian13:nonroot@sha256:cb87acc574abe719b6e5c547384defa2f7b1f24af9605d3f618cf852495d8be2 AS runtime
COPY --from=build /out/queryembed /out/healthcheck /app/
COPY --from=build --chmod=0644 /src/work/native/linux-arm64/libonnxruntime.so /src/work/native/linux-arm64/libtokenizers.a /opt/native/
USER 65532:65532
EXPOSE 8091
HEALTHCHECK --interval=30s --timeout=3s --start-period=120s --retries=3 CMD ["/app/healthcheck", "http://127.0.0.1:8091/healthz"]
ENTRYPOINT ["/app/queryembed"]
CMD ["--model-dir", "/models", "--ort-lib", "/opt/native/libonnxruntime.so", "--listen", "127.0.0.1:8091"]
