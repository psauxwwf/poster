FROM golang:1.26-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/poster ./cmd/poster


FROM ubuntu:24.04 AS runtime

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

ENV DEBIAN_FRONTEND=noninteractive \
    LANG=C.UTF-8 \
    LC_ALL=C.UTF-8 \
    TZ=UTC \
    UV_INSTALL_DIR=/usr/local/bin \
    PATH="/poster:/poster/.venv/bin:${PATH}"

WORKDIR /poster

RUN ln -snf "/usr/share/zoneinfo/${TZ}" /etc/localtime && echo "${TZ}" > /etc/timezone

RUN apt-get update && \
    apt-get install --yes --no-install-recommends \
        curl=* \
        ca-certificates=* \
        iproute2=* \
        libpcap0.8t64=* && \
    rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

RUN curl -LsSf https://astral.sh/uv/install.sh | sh

COPY pyproject.toml uv.lock ./
RUN uv sync --no-cache

COPY --from=builder /out/poster ./poster

ENTRYPOINT ["./poster"]
