# Q-3 运行生命周期与安全边界自检记录

| 项 | 结果 |
|---|---|
| 状态 | 用户于 2026-11-17 验收通过 |
| 关联 | 前后端代码质量专项 Q-3；F-06/F-13/F-18；AC-M2-9/11、AC-U-42 回归 |
| 范围 | 运行启动准备、活动事件分发、终态清理；运行工作区创建与有界查询职责拆分；冻结执行快照范围核对 |

## 实现

- 将运行启动流程拆分为冻结快照/工作区/上下文准备与 MCP、权限、Driver 资源启动；将 Agent 事件租约管理与事件分发分开，并把终态资源清理、最终记录构造和持久化重试提取为独立步骤。
- `RunScope` 从冻结的 `RunExecutionSnapshot` 构造；启动前核对工作区 `ProjectID/TreeID/NodeID/ChainPath/InputNodeIDs` 与快照身份，拒绝不匹配的 workspace。测试还验证活动工程变化不修改已冻结的 DAG 输入快照。
- 将 `View` 的路径、工程和快照构造留在 `view.go`；有界文件读取、列举、搜索及逻辑路径校验移至 `view_read.go`。路径解析、重解析点/junction 拒绝、未选输入隔离、目标节点唯一写入和共享 `runtime/` 只读规则保持不变。
- 补充故障注入覆盖 MCP 启动失败、部分 MCP 配置准备失败后的清理、MCP 停止失败、配置清理失败和终态记录写入失败后的重试；验证失败终态下运行锁与全局 gate 仍释放。
- 未更改 RPC、DTO、持久化格式、MCP 工具权限或 UI；未调用真实 CLI、外网或桌面窗口。

## 验证

以下检查离线执行并通过：

```bash
AUDIOUI_LIVE_CLI=0 GOPROXY=off GOSUMDB=off go test ./internal/service/run ./internal/infra/workspace ./internal/infra/paths ./internal/infra/agent/mcp
AUDIOUI_LIVE_CLI=0 GOPROXY=off GOSUMDB=off go test -race ./internal/service/run ./internal/infra/workspace ./internal/infra/paths ./internal/infra/agent/mcp
GOPROXY=off GOSUMDB=off AUDIOUI_LIVE_CLI=0 go test . ./internal/...
GOPROXY=off GOSUMDB=off go vet . ./internal/...
git diff --check
```

`gofmt` 已运行于本任务修改的 Go 文件。全量测试涵盖原有运行事件序号、待决请求、运行租约和路径隔离回归。未运行前端门禁、Wails 构建、GUI/浏览器或真实 Agent/CLI；本任务不涉及前端及生成绑定。

## 未覆盖与验收边界

Q-3 已于 2026-11-17 通过用户验收；本记录仅覆盖 Q-3 的实现与自检，不代表代码质量专项整体完成。Q-4～Q-7 未开始，M3 仍须等待代码质量专项总出口门禁。
