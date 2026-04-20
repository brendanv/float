FROM debian:bookworm-slim AS hledger
ARG TARGETARCH
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates \
    && HLEDGER_ARCH=$([ "$TARGETARCH" = "arm64" ] && echo arm64 || echo x64) \
    && curl -fsSL "https://github.com/simonmichael/hledger/releases/download/1.52/hledger-linux-${HLEDGER_ARCH}.tar.gz" \
       | tar xz -C /usr/local/bin/ \
    && hledger --version

FROM oven/bun:1.3.9-debian AS web-build
WORKDIR /app
COPY web/package.json web/bun.lock ./web/
RUN cd web && bun install --frozen-lockfile
COPY proto/ ./proto/
COPY web/ ./web/
RUN cd web && bunx buf generate --template buf.gen.yaml ../proto \
    && bun run build

FROM golang:1.26.1-bookworm AS go-build
COPY --from=bufbuild/buf:1.66.1 /usr/local/bin/buf /usr/local/bin/buf
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download \
    && go install google.golang.org/protobuf/cmd/protoc-gen-go@latest \
    && go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest
COPY . .
RUN buf generate
COPY --from=web-build /app/internal/webui/dist/ ./internal/webui/dist/
RUN CGO_ENABLED=0 go build -o /floatd ./cmd/floatd/

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=hledger /usr/local/bin/hledger /usr/local/bin/hledger
COPY --from=go-build /floatd /usr/local/bin/floatd
RUN useradd --create-home --shell /bin/bash float
USER float
VOLUME /data
EXPOSE 8080 2222
ENTRYPOINT ["floatd"]
CMD ["--data-dir", "/data"]
