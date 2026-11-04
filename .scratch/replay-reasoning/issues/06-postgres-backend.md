# 06: Postgres 后端与内存降级

**What to build:** 回放条目可以存进 Postgres，多实例部署时共享。DSN 由插件自己的配置给出，不复用宿主连接。没有配置 DSN 时自动降级到内存，功能仍可用，但管理界面必须明确显示当前是内存模式，不能只写日志。

**Blocked by:** 02

**Status:** ready-for-agent

- [ ] `backend: postgres` 且配置了 DSN 时，条目写入并从 Postgres 读回
- [ ] `backend: postgres` 但 DSN 为空时配置校验报错
- [ ] `backend: memory` 时不校验 DSN
- [ ] 无 DSN 且允许降级时，功能在内存模式下可用，重启即丢
- [ ] 管理界面显示当前存储模式（postgres / memory）
- [ ] 文档写明内存模式重启后条目丢失，以及回放库含推理明文、应按凭据等级保护
- [ ] 真实 Postgres 的连接验证留给人工 e2e；自动化测试用内存实现验证同一套行为
