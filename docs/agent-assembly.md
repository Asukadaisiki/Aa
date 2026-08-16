# Agent 组装与会话格式

## 1. 运行时组装

`core.Agent` 是后端对外的编排边界，依赖关系如下：

```text
Agent
 ├── Loop       一个用户 turn 的生命周期与工具循环
 │    ├── Client  一次 provider 请求与流式事件转换
 │    └── Session 当前会话的有序消息历史
 └── Session    可导出、可持久化的会话快照
```

推荐在进程启动或会话创建时实例化一次 Agent：

```go
agent, err := core.NewAgentFromConfig(
    cfg,
    tools.NewRegistry(),
    tools.Context{WorkDir: workDir, Mode: tools.PermissionModeAutonomous},
    nil,
)
if err != nil {
    return err
}

events, err := agent.Run(ctx, input)
if err != nil {
    return err
}
for event := range events {
    // text_delta / reasoning_delta: 立即推送给客户端
    // tool_call / tool_result: 推送工具状态
    // done: 本轮结束
    // error: 本轮失败或被取消
    _ = event
}
```

如果 provider 已经在上层创建，也可以使用 `core.NewAgent(client, session)` 注入。

## 2. 一次 Run 的状态流转

```text
Run(input)
  -> append user message
  -> Client.Post(session snapshot)
  -> stream deltas to caller
  -> receive complete response
  -> append assistant message
  -> no tool_calls: emit done and exit
  -> has tool_calls:
       emit tool_call
       execute tool
       emit tool_result
       append tool message
       repeat Client.Post
```

`done` 是本次用户 turn 的终止信号。流结束后才能开始同一个 Agent 的下一次 Run；Loop 会保证同一个 Agent 的会话消息不会交叉写入。

## 3. Session 的规范化消息格式

Session 保存的是 provider-neutral、JSON-ready 的消息数组，字段沿用 `provider.Message`：

```json
[
  {"role":"system","content":"你是一个可靠的 coding agent"},
  {"role":"user","content":"读取 notes.txt"},
  {
    "role":"assistant",
    "content":"",
    "reasoning_content":"...",
    "tool_calls":[
      {"id":"call_1","name":"read","arguments":{"path":"notes.txt"}}
    ]
  },
  {
    "role":"tool",
    "name":"read",
    "tool_call_id":"call_1",
    "content":"{\"content\":[{\"type\":\"text\",\"text\":\"...\"}]}"
  },
  {"role":"assistant","content":"文件内容是……"}
]
```

约束：

- 流式 delta 不进入 Session；一轮完成后只追加一条完整 assistant 消息。
- assistant 的 `tool_calls` 与后续 tool 消息必须保留原始 `id` / `tool_call_id` 对应关系。
- 一个 assistant 消息可以包含多个 tool call，随后为每个 call 追加一条 tool 消息。
- 工具结果的 `content` 使用 JSON 字符串保存完整 `tools.Result`，避免丢失 `details`。
- 下一轮请求始终使用 `Session.Messages()` 的完整有序快照，provider 不保存状态。

会话持久化时直接对 `agent.Messages()` 做 `json.Marshal` 即可；恢复时将解码后的 `[]provider.Message` 传入 `core.NewSession(messages...)`。
