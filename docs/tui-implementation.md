# TUI 实施记录

## 参考交互

参考 pi-agent 的 packages/tui/test/chat-simple.ts：

- 顶部欢迎信息和状态区；
- 对话记录区；
- 提交后显示 loading 状态；
- assistant 文本按流式增量更新；
- slash command 清理或退出。

Aa 采用 Go 实现交互式终端层，raw 模式使用 `golang.org/x/term`，非交互环境和 `-no-ansi` 保留 stdin 行模式。交互路径将完整逻辑帧映射到固定高度视口，只更新发生变化的终端行，并以 16ms 合并高频流式更新。

## 事件映射

message.Event 已经具备 TUI 所需的边界：

- reasoning_delta -> reasoning 状态；
- text_delta -> assistant 当前消息；
- tool_call / tool_result -> 工具状态；
- usage -> 当前轮次和累计 token/cache 统计；
- done / error -> 本轮结束或错误。

/clear 不仅清空画面，也调用 core.Agent.ClearSession 清空内存消息，避免用户看到的会话与下一次请求携带的历史不一致。

启动入口默认直接读取 configs/config.example.yaml；provider API key 沿用现有逻辑，YAML 中有 api_key 时优先使用，否则回退到环境变量。

## 当前边界

TUI 已具备原始输入、光标编辑、多行输入、历史、slash/file 补全、自动滚动视口、差分渲染和 context/cache 状态栏。Aa 项目本身的 `agent/protocol`、`agent/transport/websocket` 仍为空壳，`cmd/a2a_agent` 仍是 stdin 行模式；这些属于服务化接入层，不在本次 TUI 范围内。
