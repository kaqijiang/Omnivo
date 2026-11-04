# Omnivo 请求与响应内容留存

这是基于 New API 的可选内容留存功能。网关将 HTTP 文本请求和 SSE 响应分段采集，经 Kafka 发送到独立 Go 归档服务；正文压缩、加密后写入 S3 兼容对象存储，独立数据库保存查询索引。平台原有主数据库和计费日志不迁移。

首版覆盖 `POST /v1/chat/completions`、`POST /v1/responses`、`POST /v1/messages`。采集的是鉴权、解压后的客户端请求实体，以及网关成功写出的响应字节；不保存认证头、Cookie 或 URL 查询参数。WebSocket、独立图片/音视频接口、原生 Gemini 路径及每次上游原始报文不属于这版范围。客户端 JSON 内主动携带的图片 Base64、工具参数或其他敏感内容仍属于正文，受正文大小限制；不要将“未保存认证头”理解为自动识别正文内的所有秘密。

默认关闭。只有有效的 root 网站登录会话能查看正文，个人访问令牌不能读取。已开启的采集与用户响应并行，不能作为严格无损审计：进程在提交队列前崩溃、达到容量上限或投递失败时可能缺失。界面区分生成结果、截断与归档状态，未收到完整记录不能显示成已完整保存。

## 1. 单服务器试运行

`compose.capture.yml` 是可选叠加配置，要求 Docker Compose 2.24.4 或更新版本，使用：

- Apache Kafka 4.1.1，KRaft 单 broker，3 个分区。
- PostgreSQL 16.15，独立的 `capture` 索引库。
- SeaweedFS 3.97，供本地或单服务器试运行的私有 S3 兼容存储。
- 单独构建的 `content-archiver` 进程。

第三方服务镜像均固定版本和 digest。Kafka、数据库、对象存储以及归档进程不发布宿主端口；应用通过单独的 Docker 内部网络访问。初始化服务会修正 Kafka 数据卷的属主并幂等创建桶和主题。该配置不具有机器故障容错，不要将同一服务器上的多个容器视为高可用集群。

归档镜像使用静态 Go 二进制与非 root 用户，包含 CA 证书和时区数据，无需在运行时安装软件。构建需要 BuildKit；受控网络若使用企业 CA，可以通过 Docker BuildKit 的 `build_ca` secret 提供证书文件，不需要关闭 TLS 校验，也不会把该临时 CA 写入最终镜像。

先在仓库根目录生成一次配置，不覆盖已有密钥：

```bash
python3 - <<'PY'
from pathlib import Path
import base64
import os
import secrets

target = Path('.env.capture')
if target.exists():
    raise SystemExit('.env.capture already exists; keep the existing keys')
values = {
    'CONTENT_ARCHIVE_KEY': base64.b64encode(secrets.token_bytes(32)).decode(),
    'CAPTURE_POSTGRES_PASSWORD': secrets.token_hex(24),
    'CONTENT_ARCHIVE_S3_ACCESS_KEY': secrets.token_hex(12),
    'CONTENT_ARCHIVE_S3_SECRET_KEY': secrets.token_hex(24),
    'CAPTURE_KAFKA_CLUSTER_ID': base64.urlsafe_b64encode(secrets.token_bytes(16)).decode().rstrip('='),
}
lines = Path('.env.capture.example').read_text().splitlines()
for i, line in enumerate(lines):
    key = line.partition('=')[0]
    if key in values:
        lines[i] = key + '=' + values[key]
fd = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
with os.fdopen(fd, 'w') as handle:
    handle.write('\n'.join(lines) + '\n')
print('Created .env.capture; keep a protected backup')
PY
```

不要打印完整 `docker compose config`、提交 `.env.capture`，或在更新时重新生成这些值。加密密钥用于队列消息和正文解密，丢失后无法恢复旧内容；更换数据库密码也不会自动修改已有 PostgreSQL 数据卷中的密码。示例生成的字母数字凭据同时满足 PostgreSQL URI 和本地 S3 配置的格式限制。

本地部署沿用原来的 Compose 项目名和应用数据卷：

```bash
docker compose --env-file .env.capture -p omnivo-local \
  -f compose.local.yml -f compose.capture.yml config --quiet

docker compose --env-file .env.capture -p omnivo-local \
  -f compose.local.yml -f compose.capture.yml up -d --build --wait
```

已有 HTTPS 服务器部署则沿用 `omnivo-server` 项目：

```bash
docker compose --env-file .env.server --env-file .env.capture -p omnivo-server \
  -f compose.server.yml -f compose.capture.yml config --quiet

docker compose --env-file .env.server --env-file .env.capture -p omnivo-server \
  -f compose.server.yml -f compose.capture.yml up -d --build --wait
```

如果之前使用自定义 `-p`，这里也使用同一个名字。不要用新项目名启动后误以为原数据库已丢失。

应用对归档依赖使用 `required: false`，归档进程自身仍要求依赖健康。若归档依赖启动失败，完整 `up --wait` 命令仍可能报告失败，应检查应用自身是否正常。需要优先恢复主服务时，可沿用原项目名和配置单独启动应用，不等待归档服务：

```bash
docker compose --env-file .env.capture -p omnivo-local \
  -f compose.local.yml -f compose.capture.yml up -d --no-deps omnivo
```

HTTPS 部署使用相同的 `--no-deps omnivo`，并保留前述两个 env 文件、`-p omnivo-server` 和 `compose.server.yml`。归档连接初始化仍受应用自身的超时控制；主服务恢复不代表内容留存恢复。如果启动时索引不可用而使留存组件初始化失败，修复依赖后需重新启动应用完成初始化，再检查留存就绪状态和测试请求。

启动依赖不会自动开启正文采集。root 登录后，在系统设置的请求内容留存区域检查网关配置就绪状态，设置保留天数与单方向正文上限，再开启并保存。`ready` 表示网关完成了留存组件初始化，不表示后台归档进程健康或所有内容已保存；应结合 Kafka 积压、归档进程状态和一条实际测试请求检查整条链路。发送新的文本 JSON 或流式请求后，从该条用量日志详情按需读取请求和响应。尚未写入、未采集或已过期时可能查不到内容；旧请求无法补录。`archiving` 表示尚未确认归档完成，也可能是进程异常退出、结束事件丢失而留下的不完整记录，不能单凭该状态判断用户请求仍在执行。

## 2. 写入与容量

每条请求按顺序产生开始、数据和结束事件。响应仍即时 Flush，采集数据按字节阈值或短定时合并发送。后台持续归档有限大小的批次，不等待整条长流结束。正文存储成功、索引事务成功后才确认 Kafka 消费位置；失败后重试，事件身份用于处理重复投递。

示例值为待压测的起点：采集器和 Kafka 生产者各自使用 32 MiB 的缓冲预算，最多 1024 个活跃采集、32 KiB 分段、100 ms 定时刷新、4 MiB 归档批次。这里不是整个功能总共只使用 32 MiB；仅上述两层预算就可合计约 64 MiB，此外还有活动状态、SDK 缓冲及压缩加密临时内存。它们不等于经过验证的并发容量，归档进程的消费者、对象读取与索引连接也需独立纳入服务器预算。

单 broker 示例主题按时间或大小先到者淘汰：默认 24 小时、每分区约 1 GiB，3 个分区约 3 GiB，另有段文件和元数据开销。Kafka 保留策略不会等消费者确认，归档落后超过此窗口会丢失尚未归档的内容。投产前按实际正文 MB/s、可容忍归档停机时间及磁盘余量调整，并监控消费者积压；不要把主题保留期与正文保留天数混为一谈。

正文读取先检查请求自身的有效期，到期立即拒绝读取。归档按 `expires_at` 的分钟桶分批，同一对象内的到期时间相差小于一分钟；不会把保留 7 天和 365 天的内容混在同一对象。物理删除在该对象最长有效期之后另留 5 分钟宽限，由独立任务以有限并发持续分页清理，无到期项或发生故障时等待或退避。因此还会有分桶、宽限和调度延迟；积压或存储故障可能继续推迟删除，这不是严格的物理删除时限。预留但未完成的对象同样进入清理；备份和快照需单独安排保留策略。关闭采集只停止新请求记录，不立即删除尚未到期内容。数据盘满可能同时影响同机应用，应为归档卷设置磁盘监控及独立资源预算。

## 3. 多服务器生产部署

继续使用同一套应用和归档镜像，把下列连接参数设为受控基础设施的地址，不必使用上述单服务器 Kafka、PostgreSQL 或 SeaweedFS 服务：

| 参数 | 用途 |
| --- | --- |
| `CONTENT_ARCHIVE_BROKERS` | Kafka bootstrap 地址，逗号分隔 |
| `CONTENT_ARCHIVE_TOPIC` / `CONTENT_ARCHIVE_GROUP` | 专用主题及归档消费者组 |
| `CONTENT_ARCHIVE_KEY` | 共用的标准 Base64 编码 32 字节内容加密密钥 |
| `CONTENT_ARCHIVE_INDEX_DSN` | 独立索引库 DSN；生产建议 PostgreSQL |
| `CONTENT_ARCHIVE_S3_ENDPOINT` | S3 兼容端点；AWS S3 使用 SDK 对应区域端点 |
| `CONTENT_ARCHIVE_S3_REGION` / `CONTENT_ARCHIVE_S3_BUCKET` | 区域和私有桶 |
| `CONTENT_ARCHIVE_S3_ACCESS_KEY` / `CONTENT_ARCHIVE_S3_SECRET_KEY` | 有相应桶权限的服务凭据 |
| `CONTENT_ARCHIVE_S3_PATH_STYLE` | S3 兼容存储通常设为 `true`，按供应商要求配置 |
| `CONTENT_ARCHIVE_KAFKA_TLS` / `CONTENT_ARCHIVE_KAFKA_CA_FILE` | Kafka TLS 及可选受信 CA 文件 |
| `CONTENT_ARCHIVE_SASL_MECHANISM` / `CONTENT_ARCHIVE_SASL_USERNAME` / `CONTENT_ARCHIVE_SASL_PASSWORD` | Kafka 身份验证；支持范围以配置校验为准 |

应用和归档进程需要一致的队列、加密密钥、索引和对象存储配置。生产配置应单独定义这两个服务的连接参数与必要挂载，不应叠加 `compose.capture.yml` 后仍依赖本地单 broker。保持 Kafka 内网访问及 TLS/SASL、对象桶私有访问，使用独立限权凭据。加密不能替代身份验证和网络隔离。

需要单个 broker 故障后继续写入时，典型配置是跨机器/故障域部署 3 broker、复制因子 3、`min.insync.replicas=2`、生产者 `acks=all`，并正确配置 KRaft 控制器仲裁。增加消费者可以分担不同分区，但也要同时核实对象存储、索引数据库与网络的持续吞吐。单机版本的主题复制因子和控制器配置不能直接搬到此环境。

## 4. 更新、检查和停用

更新前备份 `.env.capture`、独立索引库和对象存储，保留 Kafka 数据与消费者位置。按原项目名使用同样的 Compose 文件更新应用和归档镜像；不要执行 `down -v`。

检查状态时只输出必要服务信息，避免导出包含秘密的完整配置：

```bash
docker compose --env-file .env.capture -p omnivo-local \
  -f compose.local.yml -f compose.capture.yml ps

docker compose --env-file .env.capture -p omnivo-local \
  -f compose.local.yml -f compose.capture.yml logs --tail=100 content-archiver
```

排查依次看网关的采集跳过/投递失败计数、Kafka 消费积压、归档进程退出或重试、索引连接及对象写入。配置、权限和容量故障需修复后恢复消费，不应通过随意改消费者组、重置 offset 或删主题掩盖。

归档进程被强制终止后，Kafka 消费者组可能需要等会话超时和分区重分配，重启不等于立即开始消费。此时应监控 lag 是否继续增长、分区是否已分配以及之后是否恢复推进。优雅停止通常恢复更快；部署配置给归档进程保留了停止宽限时间，避免把正常更新变成强制中断。

停用时先在 root 设置中关闭采集，再让已有调用与队列中的归档完成。关闭相关容器不会自动清除历史内容；继续查看旧内容仍需要索引、对象存储及原加密密钥。若要完全退回原部署，用原基础 Compose 更新应用并保留归档数据，另安排过期清理，不删除平台原 `data` 卷。

上线验收应包含：JSON/SSE 字节与首包行为、长流期间持续落盘、断连/重试/截断、真实 Kafka 重放与消费者重启、对象成功但索引失败的恢复、超限及存储不可用、root 与非 root 的读取边界，以及关闭采集后的正常调用。硬件容量只依据实际混合流量压测判断，不根据组件名称承诺 QPS。

## 5. 验证与权限参考

此次实现的权限检查参考 [OWASP ASVS 5.0.0](https://github.com/OWASP/ASVS/tree/v5.0.0) 的 8.2.1–8.2.3、8.3.1–8.3.2，以及 [Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)、[Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)、[Authorization](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)、[Logging](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html) Cheat Sheets。

聚焦回归覆盖有效 root 会话读取、普通用户/管理员/PAT 拒绝、请求关联校验、读取期间降权或注销、`Cache-Control: no-store`、前端按身份隔离及关闭/错误后清除明文。这里描述本功能的检查范围，不构成全站 ASVS 认证声明。

数据库兼容性验证应使用独立真实 SQLite、MySQL、PostgreSQL 测试库，分别检查新建与已有数据升级、重复迁移及幂等重放；测试代码因外部环境变量未配置而跳过时不能计作验证通过。生产环境也需要复查 Kafka 的复制/确认策略、桶权限、数据库权限及磁盘预算，不能只凭一次本地启动判定高可用。

### 本次实现验证（2026-11-04）

以下检查已在隔离环境完成，未使用真实客户正文或供应商账户：

- `go test ./...` 全部通过；`go test -race ./controller ./router ./service -run 'Test(RequestContent|ResponsesProtocolRoute|HostProtocolRegistry)' -count=1` 通过；改动涉及的 Go 包通过 `go vet`，网关和归档进程均完成静态编译。
- 在设置独立测试库的 `CONTENT_ARCHIVE_TEST_MYSQL_DSN`、`CONTENT_ARCHIVE_TEST_POSTGRES_DSN` 后，执行 `go test -race ./pkg/contentarchive -run '^TestContentArchiveIndex' -count=1 -v`。真实 **SQLite 3.50.4、MySQL 5.7.44、PostgreSQL 16.15** 各 9 组索引行为测试通过，包括空库及保留既有无关表/数据的迁移、重复迁移、两个连接同时初始化、重放去重、乱序、过期清理和超时。
- 配置隔离 Kafka、S3 和 PostgreSQL 后，执行 `CONTENT_ARCHIVE_INTEGRATION_TEST=1 go test -race ./pkg/contentarchive -run '^TestArchive' -count=1 -timeout=100s`，验证真实归档、结束事件前位点推进、新消费组重放及停止。之后新增的清理回归也通过 race 检查：205 个过期对象中 1 个持续删除失败，其余 204 个能持续清空，并发删除最多 4 个且可取消。
- 真实 HTTP 网关验证三个接口各自的 JSON/SSE 请求与响应字节、长流中途可读、截断不改变客户响应、root 权限和注销，以及 16 路混合并发。共核验 26 条请求、105 个队列事件，无重复或串用；停止消费者期间模型调用继续，重启后积压恢复。测试中优雅重启约 0.32 秒恢复，强制终止后约 45.12 秒恢复；后者包含 Kafka 会话超时和重新分配时间，不是立即恢复保证。
- 在 `web/` 执行 `bun run typecheck`、`bun run build`，并运行两份 `request-content.test.tsx` 的 18 项测试，均通过；改动文件通过 lint。七语言、桌面/手机及暗色主题的 9 种浏览器组合共 45 项检查通过，使用合成内容验证分页、复制、权限错误清空及布局。
- `Dockerfile.content-archiver` 从源码实际构建通过，使用只读模块缓存代理绕过本验证环境的代理 DNS 限制，仍执行 `go.sum` 校验。归档镜像以非 root、只读根文件系统运行检查通过。完整 Compose 叠加配置启动通过，root 状态为 `configured=true`、`ready=true`、`enabled=false`，空正文查询返回 `success/null`；初始化可重复，依赖无宿主公开端口。此次网关容器使用最终本地编译二进制组成的验证镜像，**没有重新完成主应用原 `Dockerfile` 的全源码构建**。测试容器与测试卷随后清理。

上述并发测试检查正确性，不代表生产吞吐量或延迟承诺。实际容量仍需以部署机器、模型响应长度、流式比例和归档故障时长进行压测；该实现优先保持模型接口可用，缓冲耗尽或投递失败时可能出现不完整留存。
