# 07: 流式捕获与标记下发

**What to build:** 流式请求也能带回放标记，而不只是非流式。按 01 的结论决定路径：若 `ResponseBeforeTranslator` 对流式逐 chunk 调用、且标记能在 `finish_reason` 之前独立下发，就走主路径；否则按回退顺序处理。

回退顺序：① 只在非流式生效，并在管理界面标明；② 改用 `StreamChunkInterceptor` 在下游 chunk 上补标记，此时上游原始 reasoning 已不可见，需改为旁路捕获。

**Blocked by:** 01（流式调用形态）、02

**Status:** ready-for-agent

- [ ] 流式请求结束后，客户端拿到的助手内容里带回放标记
- [ ] 标记出现在 `finish_reason` 之前，不会掉在流结束后
- [ ] 流式与非流式两条链路都能在下一轮还原出 reasoning item
- [ ] 走回退路径时，管理界面明确显示当前只支持非流式
- [ ] 流式下捕获失败不影响响应本身（fail-open）
- [ ] `stream` 与 `nonstream` 两个开关各自生效，全关时配置校验报错
