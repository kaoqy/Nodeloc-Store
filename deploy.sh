#!/usr/bin/env bash
# ============================================================
# NodeLoc Store · Docker 构建+推送脚本
#
# 用法：
#   bash deploy.sh build           # 仅构建 latest
#   bash deploy.sh build v2.0.0    # 构建并打版本标签
#   bash deploy.sh push            # 构建 + 推送 latest
#   bash deploy.sh push v2.0.0     # 构建 + 推送 v2.0.0
#
# 凭据：DOCKER_USER / DOCKER_TOKEN 或交互式输入
# ============================================================
set -euo pipefail
cd "$(dirname "$0")"

IMAGE="${DOCKER_IMAGE:-kaoqy666/nodeloc-store:latest}"

log()  { printf "\033[1;34m▶\033[0m %s\n" "$*"; }
ok()   { printf "\033[1;32m✔\033[0m %s\n" "$*"; }
err()  { printf "\033[1;31m✘\033[0m %s\n" "$*" >&2; }

login() {
  DOCKER_USER="${DOCKER_USER:-}"
  DOCKER_TOKEN="${DOCKER_TOKEN:-}"
  if [ -z "$DOCKER_USER" ] && [ -z "$DOCKER_TOKEN" ]; then
    log "登录 Docker Hub"
    read -rp "  用户名: " DOCKER_USER
    read -rsp "  Token: " DOCKER_TOKEN; echo
  fi
  echo "$DOCKER_TOKEN" | docker login -u "$DOCKER_USER" --password-stdin
  ok "登录成功"
}

build_and_push() {
  VERSION="${1:-latest}"
  local tag="${IMAGE%:*}:${VERSION}"

  log "构建 $tag ..."
  docker build -t "$IMAGE" -t "$tag" .
  ok "构建完成"

  log "推送 $tag ..."
  docker push "$tag"
  if [ "$VERSION" != "latest" ]; then
    docker push "$IMAGE"
  fi
  ok "推送完成: $tag"
}

build_only() {
  VERSION="${1:-latest}"
  log "构建 $VERSION ..."
  docker build -t "${IMAGE%:*}:${VERSION}" .
  ok "构建完成"
}

cmd="${1:-build}"
case "$cmd" in
  build)  build_only "${2:-latest}" ;;
  push)   login; build_and_push "${2:-latest}" ;;
  *)      echo "用法: bash deploy.sh [build|push] [version]"; exit 1 ;;
esac

ok "全部完成"
