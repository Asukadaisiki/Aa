# 最小 TUI 实施记录

## 参考交互

参考 pi-agent 的 packages/tui/test/chat-simple.ts：

- 顶部欢迎信息和状态区；
- 对话记录区；
- 提交后显示 loading 状态；
- assistant 文本按流式增量更新；
- slash command 清理或退出。

Aa 采用 Go 标准库实现同样的最小骨架，避免为一个入口引入额外终端依赖。每次状态变化都会清屏重绘，-no-ansi 可切换为普通日志输出。

## 事件映射

message.Event 已经具备 TUI 所需的边界：

- reasoning_delta -> reasoning 状态；
- text_delta -> assistant 当前消息；
- tool_call / tool_result -> 工具状态；
- done / error -> 本轮结束或错误。

/clear 不仅清空画面，也调用 core.Agent.ClearSession 清空内存消息，避免用户看到的会话与下一次请求携带的历史不一致。

启动入口默认直接读取 configs/config.example.yaml；provider API key 沿用现有逻辑，YAML 中有 api_key 时优先使用，否则回退到环境变量。

## 缺口

当前 TUI 是“最小可用版”，还没有 pi-agent 的原始终端输入、光标编辑、自动补全、滚动视口和差分渲染。Aa 项目本身的 agent/protocol、agent/transport/websocket 仍为空壳，cmd/a2a_agent 目前只提供 stdin 行模式；这些属于服务化接入层，不应在本次 TUI 里伪装成已完成。
