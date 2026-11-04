# 01: 回答风格注入端到端验证

**What to build:** 用真实 CPA 宿主加 mock 上游，验证已实现的回答风格注入在三种客户端格式下都能正确工作：注入、剥离、切档、守卫不误报。这是 Phase 7 的收尾，不新增功能。

覆盖：Chat 的 `system` 消息、Responses 的 `instructions`、Gemini 的 `systemInstruction`；两个家族（caveman、ponytail）各自开、同时开、都关。

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] 三种格式各发一次，mock 上游收到的系统提示末尾有标记段，段内文本与所选档位一致
- [ ] 两个家族同时开启时合并为一个注入段，caveman 在前、ponytail 在后
- [ ] 关闭后再发一次，系统提示与未启用前逐字节一致
- [ ] 连续切档 `lite` → `full`，旧段被替换而不是累积
- [ ] 开启注入后不发 `system_prompt_changed` 信号
- [ ] 未知档位在配置校验阶段报错，不静默当作 `off`
- [ ] 结论写回计划的 Phase 7 验收段
