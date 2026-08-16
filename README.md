## Aa

Aa 目前提供一个交互式 Agent TUI：复用现有流式 Agent、工具调用和会话能力，采用类似 pi-agent 的 raw 输入、光标编辑、自动补全、滚动视口和差分渲染。

### 启动

在项目目录的 Windows 终端中直接输入：

    .\Aa.cmd

如果 Aa.cmd 已经加入 PATH，也可以直接输入 Aa。配置默认读取 configs/config.example.yaml，不需要复制文件或传参；YAML 中填写 api_key 时优先使用，否则沿用环境变量回退逻辑。

可选参数：

- -permissions approval|autonomous：工具审批策略，默认 approval。
- -no-ansi：禁用清屏、颜色和重绘，便于日志环境运行。

TUI 命令：/help、/clear、/count、/stats、/quit。普通文本会进入 Agent，并实时显示流式回复。底部状态区显示上下文使用量、输入/输出 token、缓存读写和最近命中率。

### 当前查漏补缺

- 已补齐：agent/tui、cmd/a2a_agent-tui、raw 输入、光标/多行编辑、历史、补全、流式差分渲染、视口、token/cache/context 状态、工具审批提示和真实会话清空。
- 仍缺少：agent/transport/websocket 与 agent/protocol 还是占位包，当前 cmd/a2a_agent 是 stdin 行模式，还不是服务端启动入口。
- 仍未纳入：会话文件加载与 WebSocket/RPC 服务化接入。

a lite a2a agent
