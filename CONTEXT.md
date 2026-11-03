# RelayWarden

RelayWarden 是 CLIProxyAPI 的进程内插件，在请求翻译前、响应下发前和用量结算后介入，负责压缩、守卫与路由。本文件只定义这个项目自己的语言。

## 语言

### 插件职责

**压缩（compression）**：
只改写工具输出文本，不触碰系统提示、工具调用结构、推理载荷和用户消息的模块。
_Avoid_: 摘要、裁剪、提示词压缩

**守卫（guard）**：
只观察、只报告、不修改请求内容的模块。它记录信号，但不切换上游、不重试、不改写响应。
_Avoid_: 拦截器、防火墙、自动修复

**路由（routing）**：
决定请求使用哪个上游凭证，不改变请求内容的模块。
_Avoid_: 调度、负载均衡

**观察-only（observe-only）**：
守卫在 v1 的唯一工作模式。发现异常时只记录，不采取任何自动处置。
_Avoid_: 自动修复、自动切换

### 压缩

**工具输出（tool output）**：
客户端回传给模型的工具执行结果，例如 `tool_result`、`role: "tool"` 消息、`function_call_output` 或 Gemini `functionResponse`。
_Avoid_: 工具结果、返回、输出

**过滤器（filter）**：
一种针对特定工具输出形态的确定性改写器，例如 git diff、grep、构建日志、JSON。
_Avoid_: 压缩器、插件、规则

**影子模式（shadow）**：
计算压缩结果和节省字节，但把原文发给上游的模式。
_Avoid_: 预览、dry-run

**只压尾部（tail-only）**：
只压缩最后一轮用户消息之后的工具输出，保护 prompt cache 的前缀。
_Avoid_: 增量压缩、局部压缩

**压缩标记（marker）**：
压缩结果末尾的 `[relaywarden: ...]` 说明，告诉模型内容被截断。
_Avoid_: 注释、占位符

### 守卫

**信号（signal）**：
守卫对一次请求或响应作出的可观测判断，例如模型不一致、推理被剥离、缓存未命中、输出为空。
_Avoid_: 告警、错误、事件

**严重度（severity）**：
信号的等级：`low`、`medium`、`high`、`critical`。严重度只影响展示和外发阈值，不触发自动处置。
_Avoid_: 优先级、等级

**请求画像（profile）**：
请求进入上游前记录的最小事实集：模型、推理设置、最大输出、缓存键、工具名、系统提示哈希。它不是请求内容本身。
_Avoid_: 快照、缓存、上下文

**来源字段恢复（source reinjection）**：
把 before-auth 源请求里存在、after-auth 源请求里丢失的字段补回源 body。它不承诺改写最终 provider payload。
_Avoid_: 修复、重写、注入

**告警（alert）**：
信号被写入日志或发送到 webhook 的动作。告警是信号的传输方式，不是信号本身。
_Avoid_: 通知、事件

### 路由

**凭证窗口（auth window）**：
某个凭证与模型组合的滑动样本集合，包含首字延迟、错误率和降智信号数。
_Avoid_: 账号池、连接、会话

**会话亲和（session affinity）**：
同一会话优先留在同一凭证，以保护 prompt cache。
_Avoid_: 粘性、绑定

**冷启动（cold window）**：
样本数不足、还不能参与延迟评分的凭证窗口。此时调度交回 CPA。
_Avoid_: 新账号、空窗口

**交接（delegate）**：
插件调度器返回未处理，让 CPA 内置调度器继续选择凭证。
_Avoid_: 回退、放弃

### 管理界面

**线路（line）**：
管理 UI 左侧的四条状态线：压缩、守卫、路由、告警。线路只展示状态并打开对应页面；总开关在各页面内。
_Avoid_: 标签、菜单、轨道

**事件带（event tape）**：
底部按时间合并配置变更与信号的最新四条记录。
_Avoid_: 日志、时间线

**管理 key（management key）**：
调用 CPA management API 的 Bearer 凭证。页面只存在 `sessionStorage`，关闭标签页即清除。
_Avoid_: 密码、token
