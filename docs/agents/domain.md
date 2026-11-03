# 领域文档

工程技能在探索代码库前如何消费本仓库的领域文档。

## 探索前先读

- 根目录 **`CONTEXT.md`**；如果存在 **`CONTEXT-MAP.md`**，先读它，再读与当前主题相关的各上下文 `CONTEXT.md`。
- **`docs/adr/`**：读与当前工作区域相关的 ADR。多上下文仓库还要检查 `src/<context>/docs/adr/`。
- 文件不存在就**静默继续**。不要提示缺失，也不要主动建议创建；`/domain-modeling`（经由 `/grill-with-docs`、`/improve-codebase-architecture`）会在术语或决策真正确定时按需创建。

## 文件结构

单上下文（本仓库采用）：

```text
/
├── CONTEXT.md
├── docs/adr/
│   ├── 0001-replay-reasoning-through-content-markers.md
│   └── 0002-store-replay-entries-in-postgres.md
└── internal/
```

多上下文（根目录存在 `CONTEXT-MAP.md` 时）才使用每个 context 自己的 `CONTEXT.md` 与 `docs/adr/`。

## 使用术语表词汇

输出中提到领域概念时（issue 标题、重构提案、假设、测试名），使用 `CONTEXT.md` 定义的原词。不要漂移到术语表明确避免的同义词。

需要的概念不在术语表里，是一个信号：要么你在发明项目不使用的语言（重新考虑），要么这是真实缺口（记下来交给 `/domain-modeling`）。

## 标记 ADR 冲突

输出与现有 ADR 冲突时，明确说出来，不要静默覆盖：

> _与 ADR-0007（推理回放标记）冲突，但值得重开，因为……_
