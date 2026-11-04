# 02: 最小回放闭环（非流式 + 内存存储）

**What to build:** 第一条端到端竖切。非流式的 Responses 请求，插件在响应侧捕获上游的推理载荷，存进内存条目库，在返回给客户端的助手内容末尾留下 `<!--rw-replay:<25 位引用>-->`；客户端下一轮回传该标记时，插件在翻译后的上游 payload 里把标记还原成 reasoning item 并删掉标记。

这一票包含：回放条目存储的接口与内存实现、捕获、标记生成、还原、`replay` 配置节与基础校验。不含指纹、被拒记忆、Postgres、流式、淘汰上限（后续票）。

**Blocked by:** 01（位置规则与钩子形态由该票给出结论）

**Status:** resolved

- [x] 开启 `replay.enabled` 且非流式请求命中时，mock 上游收到的下一轮请求里含上一轮的 reasoning item
- [x] reasoning item 的位置符合 01 的结论（默认插在 assistant 消息项之前）
- [x] 返回给客户端的内容里，标记是该轮 `content` 的最后一段；tool-call-only 轮写成只含标记的字符串
- [x] 同一 body 处理两次，第二次不再注入（幂等）
- [x] 同一请求里同一引用出现两次，只展开第一次
- [x] 引用查不到或格式不合法：只删标记、请求正常返回、`replay_invalid` 加一
- [x] 客户端剥离 HTML 注释时功能静默退化，不报错
- [x] 功能关闭时 body 原样返回，历史里的旧标记保留
- [x] 非 Responses 格式对原样返回
- [x] 钩子内部任何异常都 fail-open，请求照常转发
- [x] `enabled` 为真但 `stream` 与 `nonstream` 都为假时配置校验报错

## Answer

实现落在 `internal/replay`：

- `Store` 接口 + `Memory` 实现（TTL 惰性过期，未做字节上限，留给 03）。
- `Engine.NormalizeResponse`：非流式、上游 Responses 时提取 `output[]` 里的 `reasoning` 项，生成 25 位引用，写入条目，把标记追加到 assistant 消息内容末尾；没有 assistant 消息时补一个只含标记的消息项。
- `Engine.NormalizeRequest`：扫描 `input[]` 里 assistant 消息的内容，提取标记，命中条目就把 reasoning 项插在该消息项之前并删标记；引用查不到只删标记并计 `replay_invalid`；同一引用只展开一次。
- 插件注册 `request_normalizer` 与 `response_before_translator` 两个能力位，新增 `request.normalize` 与 `response.normalize_before` 两个方法分支。
- 配置新增 `replay` 节与三条校验；DSN 为空且允许降级时自动退回内存。

测试：`internal/replay` 单测覆盖捕获、还原、幂等、无效引用、tool-call-only、流式跳过、过期；`main_test.go` 经 `handleMethod` 覆盖一次完整往返；`config` 覆盖三条校验。

未做：mock 上游的端到端验证（需要真实 CPA 宿主），以及 03–08 的其余条目。

## Comments

评审后补的修正（同票）：

- 捕获同时接受裸 `output` 与 Codex 执行器传入的 `response.completed` 包装事件（`response.output`）。没有这条，真实链路永远捕获不到。
- 捕获与还原都要求客户端格式为 `openai`（Chat）。原生 Responses 客户端自己携带推理项，Claude 客户端由宿主已有回放覆盖，两者都不能再插标记，否则会产生重复 reasoning item。
- 作用域校验从 `owner` 扩到 `owner + FromFormat + Model`，并写入条目。
- 格式不合法但成对的标记也会被删除，不再因为引用长度不对而原样透传。
- 内存存储强制 `max_bytes`：按 `CreatedAt` 淘汰最旧条目，单条超过上限则拒收；过期条目在写入时一并清理。
- 在流式与 Postgres 实现完成前，配置校验直接拒绝 `stream: true` 与 `backend: postgres`，避免静默空转。
- `postgres.dsn` 改为 `json:"-"`，管理接口不再回显数据库凭据。
