# Message Client、Session 与 Agent Loop 实施记录

本阶段按 `message -> client -> session -> loop` 实现：

- `agent/message.Client` 持有配置、provider 与 tool registry，`Post` 返回流式事件 channel。
- `agent/core.Session` 使用 mutex 保存内存消息历史，并返回防修改副本。
- `agent/core.Loop` 持有 Client 与 Session，顺序处理消息、tool 搜索、tool 加载、tool 执行和后续 provider 轮次。
- 首次请求只暴露 `tool_search` 与 `tool_load`，真实 tool 根据 key 渐进加载。
- 每个用户请求最多执行 8 个 provider/tool 轮次；同一个 Loop 的请求串行执行。

实现范围不包含 WebSocket、TUI 和 Session 文件持久化接入。现有 `save` tool 仍可用于显式保存消息。
