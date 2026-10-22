# 語言層 Dockerfile 參考

專案容器的 Dockerfile 基底已包含：node 22、git、curl、make、openssh-client、Claude Code CLI、防火牆、SSH helper、known_hosts。
基底結束時處於 **root** 上下文。模板會自動在語言層之後附加 `USER node` + `WORKDIR /workspace`。

AI 只需要根據偵測到的語言追加安裝指令：
- **不需要**在開頭加 `USER root`（已經是 root）
- **不需要**在結尾加 `USER node` 或 `WORKDIR`（模板自動附加）
- 需要以 node 身份執行的指令（如 `go install`）前加 `USER node`

## Go

從 `go.mod` 讀取版本號（例如 `go 1.22`）。

```dockerfile
ARG GO_VERSION=1.22.0
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
    | tar -C /usr/local -xzf -

ENV GOROOT=/usr/local/go
ENV GOPATH=/home/node/go
ENV PATH=$GOPATH/bin:$GOROOT/bin:$PATH

USER node
RUN go install golang.org/x/tools/gopls@latest \
    && go install github.com/go-delve/delve/cmd/dlv@latest \
    && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

## Python

偵測到 `requirements.txt`、`pyproject.toml` 或 `setup.py` 時追加。

```dockerfile
ARG PYTHON_VERSION=3.12
RUN apt-get update && apt-get install -y --no-install-recommends \
    python${PYTHON_VERSION} \
    python${PYTHON_VERSION}-venv \
    python3-pip \
    && rm -rf /var/lib/apt/lists/* \
    && ln -sf /usr/bin/python${PYTHON_VERSION} /usr/bin/python \
    && ln -sf /usr/bin/python${PYTHON_VERSION} /usr/bin/python3
```

## Rust

偵測到 `Cargo.toml` 時追加。

```dockerfile
USER node
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain stable
ENV PATH=/home/node/.cargo/bin:$PATH
RUN rustup component add rust-analyzer clippy rustfmt
```

## Node.js 額外工具

Base image 已包含 Node 20。偵測到 `package.json` 時可選擇性追加：

```dockerfile
RUN npm install -g pnpm turbo tsx
```
