# RelayWarden

CLIProxyAPI 的进程内插件，Go 实现，模块名 `github.com/Kyresn/relaywarden`。按职责分为若干子包：`internal/compress` 压缩工具输出、`internal/guard` 观察保真度、`internal/routing` 选择上游凭证、`internal/metrics` 计数、`internal/management` 管理接口与内嵌界面、`internal/protocol` 请求体遍历。`main.go` 是插件 RPC 入口，测试经由它分发方法。

## 常用命令

```bash
gofmt -l .                 # 格式检查
go test ./...              # 全部测试
CGO_ENABLED=1 go build -buildmode=c-shared -o plugins/relaywarden.dll .   # 构建插件
npm --prefix web run build   # 构建管理界面，产物嵌入二进制
```

## Agent skills

### Issue tracker

问题与规格以 markdown 文件存放在 `.scratch/` 下。见 `docs/agents/issue-tracker.md`。

### Triage labels

分类使用五个默认标签：`needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`。见 `docs/agents/triage-labels.md`。

### Domain docs

单上下文：根目录 `CONTEXT.md` 加 `docs/adr/`。见 `docs/agents/domain.md`。
