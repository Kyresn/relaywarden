# 02: 最小回放闭环（非流式 + 内存存储）

**What to build:** 第一条端到端竖切。非流式的 Responses 请求，插件在响应侧捕获上游的推理载荷，存进内存条目库，在返回给客户端的助手内容末尾留下 `<!--rw-replay:<25 位引用>-->`；客户端下一轮回传该标记时，插件在翻译后的上游 payload 里把标记还原成 reasoning item 并删掉标记。

这一票包含：回放条目存储的接口与内存实现、捕获、标记生成、还原、`replay` 配置节与基础校验。不含指纹、被拒记忆、Postgres、流式、淘汰上限（后续票）。

**Blocked by:** 01（位置规则与钩子形态由该票给出结论）

**Status:** ready-for-agent

- [ ] 开启 `replay.enabled` 且非流式请求命中时，mock 上游收到的下一轮请求里含上一轮的 reasoning item
- [ ] reasoning item 的位置符合 01 的结论（默认插在 assistant 消息项之前）
- [ ] 返回给客户端的内容里，标记是该轮 `content` 的最后一段；tool-call-only 轮写成只含标记的字符串
- [ ] 同一 body 处理两次，第二次不再注入（幂等）
- [ ] 同一请求里同一引用出现两次，只展开第一次
- [ ] 引用查不到或格式不合法：只删标记、请求正常返回、`replay_invalid` 加一
- [ ] 客户端剥离 HTML 注释时功能静默退化，不报错
- [ ] 功能关闭时 body 原样返回，历史里的旧标记保留
- [ ] 非 Responses 格式对原样返回
- [ ] 钩子内部任何异常都 fail-open，请求照常转发
- [ ] `enabled` 为真但 `stream` 与 `nonstream` 都为假时配置校验报错
