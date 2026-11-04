# 01: 宿主行为验证 spike

**Type:** research

**What to build:** 在写回放代码之前，把计划 E8 的七项未知项逐条验证，结论写回计划的 E8 与 E1 表格。每项给出「结论 + 依据（源码位置或实测记录）」。验证不通过的项，按计划里的回退顺序给出调整方案。

七项：

1. `ResponseBeforeTranslator` 对流式响应是逐 chunk 调用还是整体调用。
2. 流式下注入的标记片段能否在 `finish_reason` 之前独立下发。
3. 注册 `RequestNormalizer` 后是否影响未启用回放的其它格式对。
4. 25 位回放引用在库内的冲突率与所需索引。
5. 两个钩子是否携带凭据身份或部署标识。
6. 上游拒绝推理载荷时的稳定错误特征。
7. 把 reasoning item 插在 assistant 消息项之前，上游是否接受。

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] 七项均有明确结论，写在计划的 E8 里，替换掉「待验证」表述
- [x] 第 5 项结论决定指纹强度；若拿不到凭据身份，明确写出弱作用域的退化路径
- [x] 第 6 项结论决定被拒分类能否自动识别；若识别不到，写明只记录失败率对比
- [x] 第 7 项结论决定位置规则；若不接受，写明改用哪种插入位置
- [x] 第 1、2 项结论决定流式票（07）走主路径还是回退路径
- [x] 不写回放功能代码

## Answer

依据 CLIProxyAPI v8.0.51 源码：

1. **逐 chunk**。`TranslateStream` 每个 SSE 事件调用一次，`NormalizeResponseBefore` 在该事件原生翻译之前执行（`sdk/translator/registry.go`，调用方按行循环）。
2. **可行但需额外发一个 delta**。逐事件处理无法回填已发出的文本 chunk；标记要作为额外文本 delta，在推理项事件之后、`response.completed` 之前发出。穿过原生翻译的效果需 mock 实测。
3. **所有格式对都会调用**。`RequestNormalizer` 在 `TranslateRequest` 里无条件执行，非 Responses 必须原样返回。
4. **冲突可忽略**。25 位 base32 约 125 位，引用做主键即可。
5. **拿不到凭据身份**。钩子只有 `FromFormat`、`ToFormat`、`Model`、`Stream`、`Body`，响应侧另带 `OriginalRequest`、`TranslatedRequest`。指纹固定为弱作用域（协议对，Gemini 加模型）。
6. **有稳定特征**：`invalid signature in thinking block` 与 `invalid_encrypted_content`。宿主自己按此分类为 `thinking_signature_invalid`（`codex_executor_terminal.go`）。
7. **位置规则有先例**。宿主回放把推理项插在锚点项之前再追加原输入项（`insertCodexReasoningReplayTurns`）。

附带发现：宿主已有一套回放实现，但 `codexReasoningReplayEnabledForSource` 只对 Claude 源协议返回真，Chat Completions 源不在覆盖范围。它用「会话键 + 输入前缀指纹」在服务端定位，不往客户端内容写标记；该备选已记入计划 E8 的 Further Notes，不并入 v1。
