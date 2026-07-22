FROM golang:1.26-bookworm AS builder

WORKDIR /src

RUN sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/bin

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN task build


FROM ubuntu:24.04 AS runtime

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

ENV DEBIAN_FRONTEND=noninteractive \
    LANG=C.UTF-8 \
    LC_ALL=C.UTF-8 \
    TZ=UTC \
    UV_INSTALL_DIR=/usr/local/bin \
    PLAYWRIGHT_BROWSERS_PATH=/playwright \
    PATH="/poster:/poster/.venv/bin:${PATH}"

WORKDIR /poster

RUN apt-get update && \
    apt-get install --yes --no-install-recommends \
        curl=* \
        ca-certificates=* && \
    rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

RUN curl -fsSL https://github.com/psauxwwf/proxychains/releases/latest/download/proxychains.tar.gz -o /tmp/proxychains.tar.gz && \
    tar -xzf /tmp/proxychains.tar.gz -C /

RUN curl -LsSf https://astral.sh/uv/install.sh | sh

COPY pyproject.toml uv.lock .python-version ./
RUN uv sync --no-cache

# RUN playwright install chromium && \
#     rm -rf /root/.cache/* /tmp/* /var/tmp/*

COPY --from=builder /src/bin/poster ./poster

ENTRYPOINT ["./poster"]

CMD ["--serve", "--print-logs"]
