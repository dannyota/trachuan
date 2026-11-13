# Build context: repository root. Build only the production command.
FROM docker.io/library/golang:1.27.3-trixie@sha256:af1bfc2614ebb1dbd31af514b78740fd678850e76498762b425ecee4c5c6ea80 AS build
WORKDIR /src
ARG TARGETARCH
RUN test "$TARGETARCH" = arm64
COPY go.mod go.sum ./
RUN go mod download
COPY apps/server/cmd/server/ ./apps/server/cmd/server/
COPY apps/server/internal/ ./apps/server/internal/
COPY apps/server/migrations/ ./apps/server/migrations/
COPY internal/ ./internal/
COPY deploy/images/healthcheck/ ./deploy/images/healthcheck/
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/server ./apps/server/cmd/server \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/healthcheck ./deploy/images/healthcheck

FROM gcr.io/distroless/static-debian13:nonroot@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931 AS runtime
COPY --from=build /out/server /out/healthcheck /app/
USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=30s --retries=3 CMD ["/app/healthcheck", "http://127.0.0.1:8080/healthz"]
ENTRYPOINT ["/app/server"]
CMD ["serve"]
