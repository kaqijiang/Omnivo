# 在 macOS 上运行 Omnivo

这套配置从当前仓库构建 Omnivo 前端和 [new-api](https://github.com/QuantumNous/new-api) 后端，在一个容器中运行，使用 SQLite 和 Docker 命名卷保存数据。只需要 Git 和 Docker Desktop，无需另外安装 Go、Bun、数据库或 Redis。

这是你电脑上的本地部署。下面的 `localhost` 地址指你的 Mac，不是 Codex 云端工作区。默认仅允许本机访问。

## 首次启动

1. 打开 Docker Desktop，等待 Docker Engine 运行。
2. 打开 macOS「终端」，执行：

```bash
git clone --depth 1 https://github.com/kaqijiang/Omnivo.git
cd Omnivo
docker compose -f compose.local.yml up -d --build --wait
docker compose -f compose.local.yml ps
```

如果已经克隆了仓库，在已有的 `Omnivo` 目录运行 `git pull --ff-only`，然后执行上面的 Compose 命令。首次构建需要联网下载基础镜像及依赖；后续会复用 Docker 缓存。Intel Mac 与 Apple Silicon Mac 使用各自的原生架构，无需添加 `--platform`。

容器显示 `healthy` 后，在浏览器输入 `http://localhost:3000`。首次访问会进入初始化向导，请自行设置管理员账号和密码。完成初始化后即可浏览首页、文档、模型目录和控制台。

请使用 `compose.local.yml`：仓库原有 `docker-compose.yml` 使用上游发布镜像，不包含本仓库定制的 Omnivo 页面。

模型调用还需要在控制台配置供应商渠道、API Key 和可用模型；启动网站本身不会自动获得模型访问权限。请在本地管理页面录入供应商密钥，不要将密钥提交到 Git。

## 端口与日常操作

如果 3000 端口已被其他程序占用，可改用 3001：

```bash
OMNIVO_PORT=3001 docker compose -f compose.local.yml up -d --build --wait
```

随后打开 `http://localhost:3001`。使用自定义端口时，后续 `up` 命令也要保留同样的 `OMNIVO_PORT` 前缀。

查看运行状态和最近日志：

```bash
docker compose -f compose.local.yml ps
docker compose -f compose.local.yml logs --tail=100 -f omnivo
```

按 `Ctrl+C` 只退出日志查看，不会停止服务。

停止与重新启动：

```bash
docker compose -f compose.local.yml stop
docker compose -f compose.local.yml start
```

更新代码并重新构建：

```bash
git pull --ff-only
docker compose -f compose.local.yml up -d --build --wait
```

如果有自己修改的代码，先保存修改再更新。更新前建议备份数据。

## 数据与备份

数据库、应用日志和启动时生成的密钥保存在挂载到 `/data` 的命名卷中。首次启动生成密钥，后续启动复用。重新构建镜像、重启、普通 `docker compose ... down` 都会保留命名卷。不要使用 `down -v` 或在 Docker Desktop 删除该卷，否则账号、配置和密钥会一起被删除。

以下命令暂停写入，将整个数据卷备份到你 Mac 的 `~/Omnivo-backups` 目录，然后恢复服务：

```bash
mkdir -p "$HOME/Omnivo-backups"
chmod 700 "$HOME/Omnivo-backups"
docker compose -f compose.local.yml stop
docker compose -f compose.local.yml run --rm --no-deps --entrypoint tar \
  -v "$HOME/Omnivo-backups:/backup" omnivo \
  -czf "/backup/omnivo-data-$(date +%Y%m%d-%H%M%S).tar.gz" -C /data .
docker compose -f compose.local.yml start
```

确认备份命令成功后再更新。备份包含数据库及 `.secrets` 目录，应一起保留；其中可能包含渠道密钥和账号数据，请勿公开分享。

## 排查启动问题

- `Cannot connect to the Docker daemon`：先启动 Docker Desktop，并等待 Engine 就绪。
- `port is already allocated`：按上面的方式改用空闲端口。
- 基础镜像或依赖下载失败：检查 Docker Desktop 的联网或代理设置，再重试构建。
- 构建因内存不足被终止：在 Docker Desktop 的 Resources 中增加可用内存后重试。
- 页面无法打开：先查看 `ps` 和 `logs`；只有容器启动并通过健康检查后才能访问。

此配置面向本机使用；需要让其他设备或公网访问时，应另外配置服务器、域名和 HTTPS。
