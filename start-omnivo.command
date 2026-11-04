#!/bin/bash
# Compatible with the Bash 3.2 included in macOS.
set -euo pipefail
umask 077

source_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project="${OMNIVO_PROJECT:-omnivo-local}"
backup_dir="${OMNIVO_BACKUP_DIR:-$HOME/Omnivo-backups}"
old_container=""
restore_old_on_exit=0

fail() { printf '\n%s\n' "$*" >&2; exit 1; }
say() { printf '\n%s\n' "$*"; }

[ -f "$source_dir/compose.local.yml" ] || fail '找不到 compose.local.yml，请在完整的 Omnivo 仓库中运行此脚本。'
case "$project" in
  ''|*[!a-z0-9_-]*|[-_]*) fail 'OMNIVO_PROJECT 只能使用小写字母、数字、连字符和下划线，并以字母或数字开头。' ;;
esac

# Include Desktop credential helpers even when a Homebrew docker is on PATH.
export PATH="/Applications/Docker.app/Contents/Resources/bin:$HOME/.docker/bin:$PATH"
command -v docker >/dev/null 2>&1 || fail '未找到 Docker。请将 Docker Desktop 安装到「应用程序」，打开一次后重试。'

context="${OMNIVO_DOCKER_CONTEXT:-$(docker context show)}"
endpoint="$(docker context inspect "$context" --format '{{.Endpoints.docker.Host}}')"
case "$endpoint" in
  unix://*) ;;
  *) fail "当前 Docker context（$context）不是本机 Unix socket。请使用 OMNIVO_DOCKER_CONTEXT=desktop-linux bash start-omnivo.command 指定本机 Docker Desktop。" ;;
esac
docker_cmd=(docker --context "$context")
"${docker_cmd[@]}" compose version >/dev/null 2>&1 || fail 'Docker Compose v2 不可用，请更新 Docker Desktop。'

if ! "${docker_cmd[@]}" info >/dev/null 2>&1; then
  say '正在等待 Docker Desktop 启动……'
  if [ "$(uname -s)" = Darwin ]; then open -a Docker; fi
  attempts=0
  until "${docker_cmd[@]}" info >/dev/null 2>&1; do
    attempts=$((attempts + 1))
    [ "$attempts" -lt 60 ] || fail 'Docker Engine 尚未就绪。请确认 Docker Desktop 显示运行中后重试。'
    sleep 2
  done
fi

compose=("${docker_cmd[@]}" compose --project-name "$project" --project-directory "$source_dir" -f "$source_dir/compose.local.yml")
containers="$("${docker_cmd[@]}" ps -aq --filter "label=com.docker.compose.project=$project" --filter label=com.docker.compose.service=omnivo --filter label=com.docker.compose.oneoff=False)"
if [ -n "$containers" ]; then
  [ "$(printf '%s\n' "$containers" | wc -l | tr -d ' ')" = 1 ] || fail '发现多个 Omnivo 容器，请先检查 Docker Desktop，避免升级错误的实例。'
  old_container="$containers"
else
  other_projects="$("${docker_cmd[@]}" ps -a --filter label=com.docker.compose.service=omnivo --filter label=com.docker.compose.oneoff=False --format '{{.Label "com.docker.compose.project"}}')"
  if [ -n "$other_projects" ] && [ -z "${OMNIVO_PROJECT:-}" ]; then
    fail "发现其他 Compose 项目的 Omnivo：$other_projects。请设置 OMNIVO_PROJECT 为原项目名后重试，以复用原数据卷。"
  fi
fi

volume="${project}_data"
if [ -n "$old_container" ]; then
  config_files="$("${docker_cmd[@]}" inspect --format '{{index .Config.Labels "com.docker.compose.project.config_files"}}' "$old_container")"
  case "$config_files" in
    *compose.capture.yml*) fail '原实例使用了请求留存扩展配置。请按 README.capture.zh-CN.md 升级完整服务组，避免本脚本覆盖扩展配置。' ;;
  esac
  mounted_data="$("${docker_cmd[@]}" inspect --format '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Type}}:{{.Name}}{{end}}{{end}}' "$old_container")"
  [ "$mounted_data" = "volume:$volume" ] || fail "原实例 /data 使用自定义挂载（$mounted_data）。本脚本不会替换它，请沿用原 Compose 配置升级。"
  old_port="$("${docker_cmd[@]}" inspect --format '{{range (index .HostConfig.PortBindings "3000/tcp")}}{{.HostPort}}{{end}}' "$old_container")"
  if [ -z "${OMNIVO_PORT:-}" ] && [ -n "$old_port" ]; then export OMNIVO_PORT="$old_port"; fi
fi
export OMNIVO_PORT="${OMNIVO_PORT:-3000}"
case "$OMNIVO_PORT" in ''|*[!0-9]*) fail 'OMNIVO_PORT 必须是数字。' ;; esac
[ "$OMNIVO_PORT" -ge 1 ] && [ "$OMNIVO_PORT" -le 65535 ] || fail 'OMNIVO_PORT 必须在 1 到 65535 之间。'

cleanup() {
  rc=$?
  trap - EXIT
  if [ "$restore_old_on_exit" = 1 ] && [ -n "$old_container" ]; then
    printf '\n备份阶段未完成，正在重新启动原容器……\n' >&2
    "${docker_cmd[@]}" start "$old_container" >/dev/null || true
  fi
  exit "$rc"
}
trap cleanup EXIT

say "使用本机 Docker context：$context；Compose 项目：$project；端口：$OMNIVO_PORT"
say '正在构建最新 Omnivo（首次需下载依赖，可能需要数分钟）……'
"${compose[@]}" build omnivo

if "${docker_cmd[@]}" volume inspect "$volume" >/dev/null 2>&1; then
  mkdir -p "$backup_dir"
  chmod 700 "$backup_dir"
  backup_dir="$(cd "$backup_dir" && pwd)"
  backup_name="${project}-$(date +%Y%m%d-%H%M%S)-$$.tar.gz"
  if [ -n "$old_container" ]; then
    was_running="$("${docker_cmd[@]}" inspect --format '{{.State.Running}}' "$old_container")"
    if [ "$was_running" = true ]; then
      restore_old_on_exit=1
      "${docker_cmd[@]}" stop --time 60 "$old_container" >/dev/null
    fi
  fi
  writers="$("${docker_cmd[@]}" ps -q --filter "volume=$volume")"
  [ -z "$writers" ] || fail '还有其他运行中的容器挂载同一数据卷，无法保证备份一致性；本次升级已停止。'
  say "正在备份数据库、渠道配置和密钥到 $backup_dir/$backup_name"
  "${docker_cmd[@]}" run --rm --network none --read-only \
    --mount "type=volume,src=$volume,dst=/data,readonly" \
    --mount "type=bind,src=$backup_dir,dst=/backup" \
    --entrypoint /bin/sh omnivo-new-api:local \
    -ec 'umask 077; tar -czf "/backup/$1" -C /data .' sh "$backup_name"
  [ -s "$backup_dir/$backup_name" ] || fail '未生成有效备份文件。'
  gzip -t "$backup_dir/$backup_name"
fi

# After replacement starts, do not automatically run the old binary against a
# database that the new version may already have migrated.
restore_old_on_exit=0
say '正在启动 Omnivo 并等待健康检查……'
if ! "${compose[@]}" up -d --wait --wait-timeout 180 omnivo; then
  "${compose[@]}" ps
  fail "启动未通过健康检查。请在 $source_dir 运行 docker --context $context compose -p $project -f compose.local.yml logs --tail=100 omnivo 查看原因。已有备份保留在 $backup_dir。"
fi
"${compose[@]}" ps
url="http://127.0.0.1:$OMNIVO_PORT/"
say "Omnivo 已启动：$url"
say '原有账号和配置保存在 Docker 数据卷；首次部署请在页面中完成管理员初始化。'
if [ "$(uname -s)" = Darwin ]; then open "$url" || true; fi
