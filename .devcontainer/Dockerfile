FROM node:22-bookworm-slim

LABEL piper.base.version="2"

# 確保以 root 執行安裝（DevContainers features 可能切換 USER）
USER root

# 基本系統工具（最穩定，極少變動）
RUN apt-get update && apt-get install -y --no-install-recommends \
    git curl wget ca-certificates openssh-client \
    ripgrep fd-find jq tree make \
    zsh sudo \
    iptables ipset \
    && rm -rf /var/lib/apt/lists/*

# 非 root 使用者設定 + 目錄準備（純設定，不依賴外部）
RUN usermod -aG sudo node \
    && echo "node ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers.d/node \
    && chmod 0440 /etc/sudoers.d/node \
    && mkdir -p /home/node/.claude && chown node:node /home/node/.claude \
    && mkdir -p /home/node/.ssh && chown node:node /home/node/.ssh && chmod 700 /home/node/.ssh

# 腳本（COPY 依賴檔案內容，放在穩定 RUN 之後）
COPY init-firewall.sh /usr/local/bin/
COPY git-ssh-helper.sh /usr/local/bin/
COPY piper-hook.mjs piper-statusline.mjs /usr/local/bin/
COPY piper-hooks.json /usr/local/share/
RUN chmod +x /usr/local/bin/init-firewall.sh /usr/local/bin/git-ssh-helper.sh \
    /usr/local/bin/piper-hook.mjs /usr/local/bin/piper-statusline.mjs \
    && echo "node ALL=(root) NOPASSWD: /usr/local/bin/init-firewall.sh" > /etc/sudoers.d/node-firewall \
    && chmod 0440 /etc/sudoers.d/node-firewall

# Claude Code CLI（最常變動，ARG 放這裡避免污染上層快取）
ARG CLAUDE_CODE_VERSION=latest
RUN npm install -g @anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}

# === LANGUAGE LAYER ===

ARG GO_VERSION=1.26.1
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
    | tar -C /usr/local -xzf -

ENV GOROOT=/usr/local/go
ENV GOPATH=/home/node/go
ENV PATH=$GOPATH/bin:$GOROOT/bin:$PATH

USER node
RUN go install golang.org/x/tools/gopls@latest \
    && go install github.com/go-delve/delve/cmd/dlv@latest \
    && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

USER node
WORKDIR /workspace
