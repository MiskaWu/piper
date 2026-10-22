#!/bin/bash
set -euo pipefail

# ============================================================
# DevContainer 防火牆與 Credential 保護腳本
# 由 postStartCommand 在容器啟動時執行
# ============================================================

# 防火牆模式：
#   open      - 不設任何 iptables 規則，所有網路流量放行（預設）
#   whitelist - 只允許連出到白名單域名
FIREWALL_MODE="${FIREWALL_MODE:-open}"

# ------------------------------------------------------------
# 1. 防火牆設定
# ------------------------------------------------------------

setup_firewall() {
    case "$FIREWALL_MODE" in
        open)
            echo "[firewall] Mode: open — 所有網路流量放行，不設限制"
            ;;
        whitelist)
            echo "[firewall] Mode: whitelist — 只允許白名單域名"
            setup_whitelist_firewall
            ;;
        *)
            echo "[firewall] 未知模式: $FIREWALL_MODE，退回 open 模式"
            ;;
    esac
}

setup_whitelist_firewall() {
    # 白名單域名列表
    # 優先從環境變數 ALLOWED_DOMAINS（逗號分隔）讀取，否則使用內建預設
    if [ -n "${ALLOWED_DOMAINS:-}" ]; then
        IFS=',' read -ra ALLOWED_DOMAINS_ARRAY <<< "$ALLOWED_DOMAINS"
    else
        ALLOWED_DOMAINS_ARRAY=(
            # Claude API
            "api.anthropic.com"
            "claude.ai"
            "statsig.anthropic.com"
            "sentry.io"

            # npm registry
            "registry.npmjs.org"
            "npmjs.com"

            # GitHub
            "github.com"
            "api.github.com"
            "raw.githubusercontent.com"
            "objects.githubusercontent.com"

            # Go proxy
            "proxy.golang.org"
            "sum.golang.org"
            "storage.googleapis.com"

            # PyPI
            "pypi.org"
            "files.pythonhosted.org"

            # Rust / crates.io
            "crates.io"
            "static.crates.io"

            # Docker Hub (用於 pull image)
            "registry-1.docker.io"
            "auth.docker.io"
            "production.cloudflare.docker.com"
        )
    fi

    # 建立 ipset
    ipset create allowed_domains hash:ip -exist

    # 解析域名並加入 ipset
    for domain in "${ALLOWED_DOMAINS_ARRAY[@]}"; do
        # 跳過註解行
        [[ "$domain" =~ ^# ]] && continue

        # 解析 IP 並加入
        ips=$(dig +short "$domain" 2>/dev/null | grep -E '^[0-9]+\.' || true)
        for ip in $ips; do
            ipset add allowed_domains "$ip" -exist
        done
    done

    # 設定 iptables 規則
    # 允許 loopback
    iptables -A OUTPUT -o lo -j ACCEPT

    # 允許已建立的連線
    iptables -A OUTPUT -m state --state ESTABLISHED,RELATED -j ACCEPT

    # 允許 DNS 查詢
    iptables -A OUTPUT -p udp --dport 53 -j ACCEPT
    iptables -A OUTPUT -p tcp --dport 53 -j ACCEPT

    # 允許白名單 IP
    iptables -A OUTPUT -m set --match-set allowed_domains dst -j ACCEPT

    # 允許內部網路（Docker bridge、容器間通信）
    iptables -A OUTPUT -d 10.0.0.0/8 -j ACCEPT
    iptables -A OUTPUT -d 172.16.0.0/12 -j ACCEPT
    iptables -A OUTPUT -d 192.168.0.0/16 -j ACCEPT

    # 拒絕其餘所有連出流量
    iptables -A OUTPUT -j REJECT --reject-with icmp-port-unreachable

    echo "[firewall] 白名單模式啟用完成，已加入 ${#ALLOWED_DOMAINS_ARRAY[@]} 個域名"
}

# ------------------------------------------------------------
# 2. Credential 保護
# ------------------------------------------------------------

protect_credentials() {
    local claude_dir="/home/node/.claude"

    if [ ! -d "$claude_dir" ]; then
        echo "[credentials] $claude_dir 不存在，跳過保護"
        return
    fi

    # 注意：bind mount 下 chmod 會影響 host 檔案
    # Claude 維護的 .credentials.json 需要 rw 權限來更新 token，不能鎖
    echo "[credentials] 保護 OAuth credentials（僅 whitelist 模式）..."
    echo "[credentials] 跳過 .credentials.json（Claude 需要寫入更新 token）"

    # 只保護明確不需要寫入的檔案
    find "$claude_dir" -name "*.oauth_backup*" -exec chmod 444 {} \; 2>/dev/null || true

    echo "[credentials] 保護完成"
}

# ------------------------------------------------------------
# 主流程
# ------------------------------------------------------------

echo "============================================"
echo " DevContainer 初始化"
echo " $(date '+%Y-%m-%d %H:%M:%S')"
echo "============================================"

setup_firewall

# Credential 保護只在 whitelist 模式下執行
# open 模式使用 bind mount，鎖權限會影響 host 且 Claude 需要寫入 token
if [ "$FIREWALL_MODE" = "whitelist" ]; then
    protect_credentials
else
    echo "[credentials] open 模式 — 跳過 credential 保護（bind mount 不適合鎖權限）"
fi

echo "[init] 初始化完成"
