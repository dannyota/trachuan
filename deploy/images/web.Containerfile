# Nitro bundles production dependencies into .output. Keep npm in the builder.
FROM docker.io/library/node:24.22.2-trixie-slim@sha256:0820eaa85e07d0490446dd01d14707e17e6108a36988cf374899cd057eb3812d AS build
WORKDIR /src
ARG TARGETARCH
RUN test "$TARGETARCH" = arm64
COPY packages/schema/ ./packages/schema/
COPY apps/web/package.json apps/web/package-lock.json apps/web/nuxt.config.ts apps/web/tsconfig.json ./apps/web/
COPY apps/web/app/ ./apps/web/app/
COPY apps/web/server/ ./apps/web/server/
COPY apps/web/public/ ./apps/web/public/
COPY apps/web/i18n/ ./apps/web/i18n/
RUN npm --prefix apps/web ci && npm --prefix apps/web run build

FROM gcr.io/distroless/cc-debian13:nonroot@sha256:cb87acc574abe719b6e5c547384defa2f7b1f24af9605d3f618cf852495d8be2 AS runtime
WORKDIR /app
COPY --from=build /usr/local/bin/node /usr/local/bin/node
RUN ["/usr/local/bin/node", "--version"]
COPY --from=build /src/apps/web/.output/ ./.output/
USER 65532:65532
ENV NITRO_HOST=127.0.0.1 NITRO_PORT=3000 NODE_ENV=production
EXPOSE 3000
# A TCP probe avoids consuming Nitro's public-page admission budget.
HEALTHCHECK --interval=30s --timeout=3s --start-period=30s --retries=3 CMD ["/usr/local/bin/node", "-e", "const s=require('node:net').connect(3000,'127.0.0.1',()=>{s.end();process.exit(0)});s.on('error',()=>process.exit(1));s.setTimeout(2000,()=>process.exit(1))"]
ENTRYPOINT ["/usr/local/bin/node"]
CMD [".output/server/index.mjs"]
