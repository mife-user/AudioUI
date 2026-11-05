# Agent MCP 配置启动竞态修复证据

| 项 | 内容 |
|---|---|
| 状态 | 修复已实现并通过离线验证；真实 CLI 联调未运行 |
| 日期 | 2026-11-05 |
| 范围 | SDK 版本探测、AudioUI 单实例启动顺序、MCP per-run 配置生命周期 |

## 原因分析

2026-11-05 的应用日志记录了 MCP 配置故障前约 5 秒内连续两次 `audio-ui starting`；第二次启动后 agent host 报告 `MCP_CONFIG_NOT_FOUND`。本机 Go SDK `claude-agent-sdk-go v0.10.0` 的启动顺序为：连接前，以传入的 CLI 路径启动 `-v` 版本检查；AudioUI 将自身 exe 作为 SDK 的 CLI 路径，并由 host 参数另行转发到原生 Claude CLI。

AudioUI 原先未识别 `-v`，SDK 探测因此进入普通 GUI 初始化。普通启动会在 Wails 单实例锁建立之前创建 Project/Run 服务并执行 `RecoverInterrupted`。此时首个进程已经保存 `starting` 记录并写好本次运行的 MCP 配置；第二个进程将其误认为上次崩溃残留，标记为 `interrupted` 并删除 per-run 配置目录。随后第一个进程的 Windows host 再检查该文件，得到 `MCP_CONFIG_NOT_FOUND`。启动前文件校验本身无法防止校验后的跨进程删除。

## 修复

- 在应用入口进程设置 `CLAUDE_AGENT_SDK_SKIP_VERSION_CHECK=1`，避免 SDK 把 AudioUI host 当成 Claude CLI 再启动一次。真实原生 CLI 仍由 `ResolveNative` 检查路径与最低版本。
- 将 `-v` 作为 `--version` 的无副作用别名，在解析应用数据目录和运行恢复之前退出。
- 将 `application.New`（Wails 单实例锁建立）移到 Project/Run 初始化及 `RecoverInterrupted` 之前；服务在锁成功取得后通过 `RegisterService` 注册。二次启动由 Wails 聚焦已有窗口并退出，不会扫描或删除活动运行资源。
- 新增启动模式回归测试：验证 SDK 的 `-v` 入口返回版本、设置探测抑制环境，并且不创建 `data`、`logs` 或 `webview2` 目录。

## 验证结果

| 检查 | 结果 |
|---|---|
| `GOTOOLCHAIN=local GOPROXY=off go vet . ./internal/...` | 通过 |
| `GOTOOLCHAIN=local GOPROXY=off go test . ./internal/...` | 通过 |
| `git diff --check` | 通过；Git 仅提示现有 Windows CRLF/LF 工作树转换提醒 |
| 离线生产构建（Go `production` tag、`main.version=0.2.0`） | 通过，输出 `bin/audio-ui.exe`；未执行会触发 `npm install` 的 Wails 任务 |
| 新构建 `bin/audio-ui.exe --version` 与 `-v` | 均输出 `AudioUI v0.2.0` |
| 新构建 `-v` 的隔离目录冒烟 | 通过；返回 0，隔离 `AUDIOUI_HOME` 下未创建 `data`、`logs`、`webview2` |
| 真实 Claude CLI/MCP/模型请求 | 未运行；项目约束要求单独明确授权，且可能访问外部服务或产生费用 |
| GUI / Wails WebView2 | 未运行；离线 Go 构建和版本模式冒烟不代表 GUI 验收通过 |

## 验收边界

代码层竞态已通过启动顺序调整与无副作用探测回归覆盖。未执行真实 CLI 端到端请求，因此本证据不代表 MCP HTTP 握手、模型调用、pending 生成或 M2-10B/M2-10C 验收完成。
