# RelayWarden

CLIProxyAPI v8 插件：工具输出压缩、中转降智观察、缓存亲和 + 延迟路由。

- 插件形态：Go C ABI 动态库（Windows `relaywarden.dll`），由 CLIProxyAPI 加载。
- 管理 UI：React + Vite，构建产物通过 `go:embed` 打进 DLL，页面在 `/v0/resource/plugins/relaywarden/ui`。
- 所有功能都有独立开关；开关写入 CPA v8 配置 API，持久化到 `config.yaml`。
- v1 对降智只记录告警，不自动切换 auth、不重试、不修改响应。

## 功能

### 1. Token 压缩

- 在 `request.intercept_before` 改写工具输出，覆盖：
  - OpenAI Responses / Codex：`function_call_output`、`custom_tool_call_output`、`local_shell_call_output`
  - OpenAI Chat：`role: "tool"` 消息
  - Gemini：`functionResponse.response`
- 14 个过滤器：git diff/status/log、grep、find、ls、tree、build/test 输出、日志去重、智能截断、编号文件、JSON 最小化、ANSI 清理。
- 默认 `tail_only`：只压缩最后一轮 user 消息之后的工具输出，保护 prompt cache 前缀。
- 默认 `shadow: true`：只统计节省字节，发送原文。确认收益后再关闭 shadow。
- 任何解析/压缩失败都返回原文；压缩结果必须非空且更小。

### 2. 防降智观察

- `request.intercept_before` 保存请求画像（模型、reasoning、max tokens、cache key、工具、system 哈希）。
- `request.intercept_after` 只做源级恢复：把 before-auth 中丢失的 reasoning、max_tokens、cache key、service_tier 等字段补回源 body；provider payload 的翻译在 executor 内部，插件不承诺改写最终上游字段。
- `response.intercept_after` + `usage.handle` 产生信号：
  - `model_mismatch`
  - `reasoning_stripped`
  - `source_param_dropped`
  - `cache_miss_streak`
  - `cache_miss_reason`
  - `service_tier_downgraded`
  - `system_fingerprint_changed`
  - `usage_invariant_violation`
  - `tools_changed`
  - `system_prompt_changed`
  - `empty_output`
  - `truncated`
  - `usage_missing`
  - `ttft_anomaly`
- 信号写入内存环形缓冲、可选 CPA 日志、可选 webhook。每个信号都有独立开关。

### 3. 提速

- `scheduler.pick` 使用每个 `auth + model` 的滑动窗口（默认 20 样本、15 分钟 TTL、最少 3 样本）。
- 评分：`0.60 * norm(TTFT) + 0.25 * error_rate + 0.15 * degrade_rate`。
- 会话亲和：同一 `execution_session_id` 优先留在同一 auth；如果该 auth 明显更慢则重新选择。
- 样本不足时返回 `Handled: false`，交回 CPA 内置调度器。
- 不注册流式逐 chunk 拦截器；流式请求只依赖 `usage.handle` 的最终 usage。

## 构建

前置：Go 1.26+、Node 20+、MinGW gcc（Windows 上 `-buildmode=c-shared` 需要 CGO）。

```bash
cd RelayWarden
npm --prefix web ci
npm --prefix web run build
CGO_ENABLED=1 go build -buildmode=c-shared -o plugins/relaywarden.dll .
```

或：

```bash
make build
```

产物：

- `plugins/relaywarden.dll`
- `plugins/relaywarden.h`（可删除）

DLL 文件名必须与配置键一致：`plugins.configs.relaywarden`。宿主会扫描 `plugins/` 以及 `plugins/<goos>/<goarch>/`。

## 安装

把 `relaywarden.dll` 放到 CLIProxyAPI 的 `plugins/` 目录，然后在 `config.yaml` 中加入：

```yaml
plugins:
  enabled: true
  dir: "plugins"
  configs:
    relaywarden:
      enabled: true
      priority: 10
      compression:
        enabled: true
        shadow: true
      guard:
        enabled: true
      routing:
        enabled: true
```

完整开关见 [config.example.yaml](config.example.yaml)。

重启 CPA。管理 UI：

```text
http://127.0.0.1:8317/v0/resource/plugins/relaywarden/ui
```

首次打开输入 CPA management key。key 只保存在当前标签页的 `sessionStorage`，关闭标签页即清除。页面本身是静态资源，不含运行数据；所有数据接口都需要 Bearer key。

管理 UI 支持中英文切换，右上角 `中 / EN`。语言偏好存在 `localStorage` 的 `relaywarden-lang`，不是密钥。

## 管理 API

插件管理路由（需要 CPA management key）：

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/v0/management/relaywarden/status` | 版本、配置 revision、聚合计数 |
| GET | `/v0/management/relaywarden/config` | 当前配置 |
| GET | `/v0/management/relaywarden/compression/stats` | 按过滤器/模型/协议的节省统计 |
| POST | `/v0/management/relaywarden/compression/preview` | `{"text":"...","format":"openai-response"}` |
| GET | `/v0/management/relaywarden/signals?limit=100` | 最近告警 |
| GET | `/v0/management/relaywarden/signals/summary` | 按 code 的计数 |
| GET | `/v0/management/relaywarden/usage` | 最近 usage 记录（诊断用） |
| GET | `/v0/management/relaywarden/routing` | auth/model 延迟窗口 |

开关写入 CPA v8 配置 API：

```bash
curl -X PATCH http://127.0.0.1:8317/v8/management/config/plugins/configs/relaywarden \
  -H "Authorization: Bearer $CPA_MANAGEMENT_KEY" \
  -H "Content-Type: application/json" \
  -d '{"compression":{"enabled":false}}'
```

CPA 会写回 `config.yaml` 并触发 `plugin.reconfigure`。

## 配置要点

```yaml
compression:
  mode: balanced        # safe | balanced | aggressive
  shadow: true          # 只测量，不改变发送内容
  tail_only: true       # 保护 prompt cache 前缀
  min_bytes: 500
  max_chars_per_result: 12000
  max_lines_per_result: 120
  dedup_threshold: 3
  protocols: { openai_chat: true, openai_responses: true, gemini: true }
  filters: { git_diff: true, grep: true, ... }

guard:
  enabled: true
  observe_only: true    # v1 锁定 true
  reinject_source_params: true
  inject_prompt_cache_key: false
  cache_miss_threshold: 3
  prompt_token_threshold: 2048
  signals: { model_mismatch: true, reasoning_stripped: true, ... }
  alerts:
    log: true
    min_severity: medium
    cooldown_seconds: 300
    webhook: { enabled: false, url: "" }

routing:
  enabled: true
  strategy: latency_cache   # latency_cache | latency_only | off
  cache_affinity: true
  window: 20
  ttl_seconds: 900
  min_samples: 3
  affinity_ttl_seconds: 1800
  weights: { ttft: 0.60, error: 0.25, degrade: 0.15 }

metrics:
  enabled: true
  max_signals: 1000

ui:
  enabled: true
  poll_interval_ms: 2000
  show_advanced: false
```

## 开发

```bash
cd web
npm install
npm run dev
```

Vite dev server 默认把 `/v0/management` 和 `/v8/management` 代理到 `http://127.0.0.1:8317`。没有 CPA 时可用 mock：

```bash
node web/dev/mock-server.mjs
```

然后打开 `http://127.0.0.1:5173/v0/resource/plugins/relaywarden/ui`，输入任意非空 key。

测试：

```bash
go test ./...
go test -race ./...
npm --prefix web run build
```

## 已知限制

- 不覆盖 Claude/Anthropic 客户端协议。
- 不做 Caveman 提示词瘦身；只保留后续设计空间。
- 不做流式逐 chunk 完整性检查；流式只依赖最终 usage。
- 不自动降级切换、不主动重试、不并行竞速。
- 看不到中转在离开 CPA 之后对请求/响应的二次改写；只能检测可观测信号。
- 管理 UI 面向本地单人，不做多用户权限、审计或远程共享部署。
- `guard.observe_only` 在 v1 固定为 true。

## 安全

- 管理 key 只存 `sessionStorage`，不写磁盘、不写日志。
- 资源页面设置 CSP：`default-src 'self'; connect-src 'self'; frame-ancestors 'none'`。
- 不引入外部字体、分析脚本或 CDN。
- webhook URL 必须是绝对 `http(s)`；发送异步、有界队列、失败不影响请求。
- 插件所有可选路径 fail-open；只有注册/配置解析失败会阻止加载。
