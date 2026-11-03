---
status: accepted
---

# 开源范围限定为可运行的 v0.1 插件，采用 MIT

## Context

仓库里同时有可运行的压缩/守卫/路由插件，以及尚未接线的推理回放与 prompt style 实验。公开仓库若把三者一起发布，读者会以为实验能力可用；同时插件依赖 CLIProxyAPI 的 SDK，需要与其许可兼容。

## Decision

- 公开仓库只包含可运行、可测试、可构建的 v0.1 插件：压缩、守卫、路由、管理 UI、文档与本地 e2e fixture。
- 推理回放与 prompt style 实验保存在本地分支 `experimental/replay-promptstyle`，不进入公开主分支。
- 采用 MIT 许可，与 CLIProxyAPI 一致；DLL 与源码同一许可。
- 公开仓库不包含 CPA 下载的 management panel、生成缓存或本地密钥材料。

## Alternatives

- **全部发布并标注 experimental**：读者仍会尝试不可用能力，issue 噪音大。
- **拆成两个仓库**：v0.1 还没有稳定的实验 API，拆仓成本高于收益。
- **Apache-2.0**：专利条款更明确，但与宿主项目的 MIT 生态摩擦更大；插件没有需要额外专利保护的核心实现。

## Consequences

- 主分支保持可构建；实验能力需要重新接线后才能发布。
- 后续若实验能力成熟，应从实验分支单独提 PR，并补测试与文档。
- 代码与 DLL 可以自由用于商业或闭源集成；作者不提供担保。
