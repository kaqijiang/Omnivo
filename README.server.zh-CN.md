# 在 Linux 服务器部署 Omnivo

适用于首次部署到一台 Linux 服务器：从本仓库构建 Omnivo 前端和 [new-api](https://github.com/QuantumNous/new-api) 后端，使用 SQLite 保存业务数据，由 Caddy 提供 HTTPS 和反向代理。模型请求转发给已配置的供应商，服务器无需 GPU。

建议从 Ubuntu 24.04 LTS、4 核 CPU、8 GB 内存、40 GB 以上磁盘开始，留出源码构建与日志空间；这不是并发容量承诺，实际规格需按流量测试。多实例部署和外置数据库迁移不在本说明范围内。

## 1. 准备服务器与域名

- 按 [Docker 官方 Ubuntu 安装说明](https://docs.docker.com/engine/install/ubuntu/)安装 Docker Engine、Buildx 和 Compose 插件，另安装 Git。`docker compose version` 需为 **2.24.4 或更新**；旧版不支持本配置的 `!reset`。
- 以下服务器命令使用可运行 Docker 的账户执行；该账户需能写入自己的 `~/apps` 目录。
- 将域名（例如 `ai.example.com`）的 A 记录指向服务器公网 IP；只有 IPv6 确实可达时才设置 AAAA 记录。首次部署使用直连 DNS，CDN 代理需另行配置。
- 云安全组和主机防火墙允许 TCP 80、443；SSH 端口按服务器设置允许管理访问。后端 3000 不对公网开放。80/443 必须没有被已有网站占用。
- 服务器需能访问代码/镜像仓库、证书机构及你选择的模型供应商。

`compose.server.yml` 的结构为：浏览器或 API 客户端 → Caddy:443 → Omnivo:3000。Caddy 的证书状态、业务数据库及应用密钥分别持久化；应用仅信任专用网络中 Caddy 的固定 IP。

## 2. 下载代码，填写域名

在服务器执行：

```bash
mkdir -p "$HOME/apps"
cd "$HOME/apps"
git clone https://github.com/kaqijiang/Omnivo.git
cd Omnivo
cp .env.server.example .env.server
chmod 600 .env.server
nano .env.server
```

将 `OMNIVO_DOMAIN=ai.example.com` 改成实际域名，只填写主机名。`.env.server` 已被 Git 和 Docker 构建上下文排除。应用签名和加密密钥在数据卷中首次生成，以后复用。

## 3. 通过 SSH 隧道先初始化管理员

首次初始化先使用本机端口模式，完成管理员创建后再开放网站。**两阶段都使用 `omnivo-server` 项目名**，确保复用同一个业务数据卷。

在服务器执行：

```bash
docker compose -p omnivo-server -f compose.local.yml up -d --build --wait
docker compose -p omnivo-server -f compose.local.yml ps
```

在自己的 Mac 或其他电脑另开一个终端，将用户名和 IP 换成实际值：

```bash
ssh -N -L 127.0.0.1:3300:127.0.0.1:3000 user@YOUR_SERVER_IP
```

保持隧道运行，浏览器打开 `http://127.0.0.1:3300`，完成初始化并自行设置管理员账号、密码。该流量通过 SSH 加密传输。完成后按 `Ctrl+C` 关闭隧道。

如果你在这台服务器已经使用其他项目名部署过 Omnivo，请先备份并规划完整数据卷迁移，不能把新的空卷当作原数据；Mac 上已有的数据也不会自动迁移到服务器。

## 4. 切换到 HTTPS 网站

确认初始化完成后，在服务器的 Omnivo 目录执行：

```bash
docker compose --env-file .env.server -f compose.server.yml config --quiet
docker compose --env-file .env.server -f compose.server.yml up -d --build --wait
docker compose --env-file .env.server -f compose.server.yml ps
```

Compose 会重建同名应用服务，保留 `omnivo-server_data` 卷，移除原宿主 3000 映射并开启安全登录 Cookie；Caddy 开始申请证书并自动续期。两个阶段不要手动更换 Compose 项目名，也不要执行 `down -v`。

打开 `https://你的域名`，重新登录。`--wait` 成功说明容器启动，不保证公网 DNS 与证书申请已经成功；还要验证：

```bash
curl --fail --show-error https://你的域名/api/status
docker compose --env-file .env.server -f compose.server.yml logs --tail=80 caddy
```

证书申请失败时核对域名解析、80/443 可达性、是否有错误的 AAAA 记录，以及现有反向代理/CDN 是否拦截了验证请求。

后台「站点设置」中将 `ServerAddress` 设置为同一个 `https://你的域名`，再设置 `SystemName=Omnivo`。这个后台地址用于邮件链接和支付回跳，不会由 `.env.server` 自动写入数据库。SDK 的 API Base URL 为 `https://你的域名/v1`；具体协议以模型详情页为准。

更换域名时同时更新 DNS、`.env.server`、后台 `ServerAddress` 和外部支付回调配置，然后重建容器。

## 5. 配置业务并验收

- 添加真实供应商渠道、密钥、可用模型及售卖价格，完成至少一次实际 API 调用和用量扣费检查。
- 配置 SMTP、站点内容及用户条款。邀请注册奖励 $1 需自行配置；首充给邀请者 $5 的配置见 [本地部署说明](README.local.zh-CN.md#好友首充邀请奖励)。
- 官网折扣需要维护有效参考价，见 [参考价格说明](README.official-prices.zh-CN.md)。部署不会自动生成“最低 5 折”模型或官方价格。
- 请求和响应正文默认不留存。如需启用，按[请求内容留存部署说明](README.capture.zh-CN.md)配置独立归档服务和存储，再由 root 开启。
- 配置支付商户和签名回调后，验证付款、入账与重复通知。当前审计发现的 **Creem 人工补单重复换算额度**、**缺少回调密钥仍可能开放充值** 两项问题尚未修复，修复并回归之前不要启用 Creem 收款。本部署配置没有修改充值逻辑。

## 6. 备份、更新和恢复

业务数据卷为 `omnivo-server_data`，必须整体备份，包含数据库和 `.secrets`。Caddy 状态分别位于 `omnivo-server_caddy_data`、`omnivo-server_caddy_config`。以下业务备份会短暂暂停应用写入，期间网站不可用：

```bash
mkdir -p "$HOME/Omnivo-backups"
chmod 700 "$HOME/Omnivo-backups"
docker compose --env-file .env.server -f compose.server.yml stop omnivo
docker compose --env-file .env.server -f compose.server.yml run --rm --no-deps \
  --entrypoint tar -v "$HOME/Omnivo-backups:/backup" omnivo \
  -czf "/backup/omnivo-data-$(date +%Y%m%d-%H%M%S).tar.gz" -C /data .
docker compose --env-file .env.server -f compose.server.yml start omnivo
```

确认备份命令成功，并将备份安全地复制到服务器之外。自动备份计划和恢复演练需在实际服务器上另行设置。备份可能包含用户数据和渠道密钥，请勿公开。

备份完成后更新代码和镜像：

```bash
git pull --ff-only origin main
docker compose --env-file .env.server -f compose.server.yml pull caddy
docker compose --env-file .env.server -f compose.server.yml up -d --build --wait
```

查看状态与应用日志：

```bash
docker compose --env-file .env.server -f compose.server.yml ps
docker compose --env-file .env.server -f compose.server.yml logs --tail=100 omnivo
```

Caddy 镜像固定到已验证的摘要，`pull caddy` 获取当前配置指定的版本，不会自动升级到任意最新版本。更新 Caddy 时需要先更新配置中的摘要，再重新验证代理与登录流程。

完整恢复应先停应用，将已确认有效的备份恢复到空的业务卷，再启动并验证管理员登录、渠道和账户余额。不要在已有数据库仍运行时直接覆盖文件，也不要只恢复数据库而漏掉 `.secrets`。

若专用网段与服务器现有网络冲突，在 `.env.server` 中同时指定新的 `OMNIVO_NETWORK_SUBNET`、`OMNIVO_PROXY_IP`、`OMNIVO_APP_IP`；两个 IP 必须位于新网段内且彼此不同。默认网段为 `172.30.90.0/24`。网站直接公开时由 Caddy 生成可信转发头，后续加 CDN 时需另行验证真实客户端 IP 和限流行为。
