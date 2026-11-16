# U8 本地 Agent 搜索能力核对

| 项目 | 结果 |
|---|---|
| 状态 | 用户确认 U8 按离线能力核对范围于 2026-11-16 完成；WebSearch 接入未做，AC-U-18 未验收 |
| 范围 | 核对 AudioUI 显式工具清单、锁定 SDK 的参数映射、本机原生 CLI 与权限策略 |
| 外网 / Agent | 未发起 Agent 会话、未访问外网、未下载内容 |

## 核对结果

- `internal/service/run/manager.go` 的 `runBuiltinTools` 仅包含 `Read`、`Glob`、`Grep`、`Write`、`Edit`、`Bash`。`runAllowedTools` 是独立的 AudioUI MCP 工具白名单，不包含通用搜索代理。
- `internal/infra/agent/claudecli/driver.go` 将 `BuiltinTools` 传给 SDK 的 `WithTools`。仓库锁定的 `claude-agent-sdk-go v0.10.0` 中，`WithTools` 保存为工具名称列表；SDK 的 CLI 序列化把它作为显式 `--tools` 列表传递。当前 AudioUI 配置因此没有显式开放 `WebSearch`。
- 本机已安装的原生 CLI 为 `2.1.316`，高于 `internal/infra/agent/claudecli/native.go` 的最低版本 `2.1.224`。CLI `--help` 说明 `--tools` 用于指定内置工具集合中的工具名称。本机安装包的 `sdk-tools.d.ts` 定义了 `WebSearchInput`（含 `query`、可选允许/屏蔽域名）与 `WebSearchOutput`；这表明该包带有相关工具类型定义，不证明 AudioUI 当前运行可调用它。
- 本机 `--help` 对 Restricted mode 明确提到 `WebFetch`，但未列出 `WebSearch` 的运行支持细节。没有依据把 `WebFetch` 与网页搜索视为等价能力。
- 对 `claude.exe --tools WebSearch --version` 的本地试探与无效工具名对照都在 `--version` 路径返回版本；该结果未启动会话或校验工具名称，**不作为 WebSearch 可用证据**。二进制字符串命中同样不构成可用性证据。

## 权限与隐私边界

- `internal/service/permission/policy.go` 对未识别工具的默认策略是交互确认；因此若未来允许 CLI 发起 `WebSearch` 调用，当前策略会进入通用确认分支，而不是自动允许。
- `internal/infra/agent/claudecli/driver.go` 生成通用工具确认时使用通用风险说明；非 Bash 工具可选择“本次运行允许”。搜索工具没有专属的“查询可能发送到外部服务”提示。按当前 permission key，未提供命令或路径的同一工具调用会共享工具级本次运行许可，不能视作逐条查询确认。
- 因此接入前仍须评审搜索提示内容与授权粒度；本轮未替用户决定权限交互，也未修改策略或 UI。
- 搜索职责应继续属于 Agent/其执行环境，不新增 AudioUI MCP 搜索代理；文件路径、Bash 门禁、PreToolUse、审计及现有 MCP 授权均不因本次核对改变。

## 范围边界与后续事项

- 用户确认 U8 以本记录的离线能力核对范围完成；该确认不代表功能接入或 AC-U-18 验收通过。
- 未验证 `WebSearch` 是否能以 `--tools` 精确名称在真实 CLI 会话中运行，也未进行任何真实搜索；该验证会建立 Agent 会话并访问外部服务，须另行明确授权。
- 未实现运行能力报告、UI 缺失能力提示、`WebSearch` 接入或专属权限文案；AC-U-18 尚未验收。后续是否纳入实现需另行规划，不计入 U8 已完成范围。
- U9 仍是最终集成门禁；U8 的离线核对结论不能代替 AC-U-18 的功能验收。

## 本地复核

在 Windows 可使用本机安装的 `claude.exe` 执行只读命令：

```powershell
& 'D:\命令行工具\NOde-js\node_global\node_modules\@anthropic-ai\claude-code\bin\claude.exe' --version
& 'D:\命令行工具\NOde-js\node_global\node_modules\@anthropic-ai\claude-code\bin\claude.exe' --help
```

复核 SDK 时检查仓库锁定的 `github.com/severity1/claude-agent-sdk-go v0.10.0` 中 `options.go` 的 `WithTools` 与 `internal/cli/discovery.go` 的 `addToolsFlag`；安装包内类型声明只能作为能力声明线索，不能替代真实运行验证。
