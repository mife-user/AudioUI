# U24 画布连接入口与关系栏清理自检

| 项目 | 结果 |
|---|---|
| 状态 | 用户于 2026-11-15 确认 AC-U-63～65 页面验收通过 |
| 范围 | AC-U-63～65；不改变 U14/U16/U19 历史验收结论 |
| 交互依据 | [U21～U24 单文件交互示意](../../design/u21-u24-chain-reconnect-demo.html) |

## 实现

- 从画布工具栏移除旧连接目标选择；移除会随选中节点出现、占用工具栏与画布间布局空间的输入关系栏及其来源替换、断开、版本选择控件。
- 保留端口鼠标新连、U23 箭头目标端改接、线身关系浮层中的断开确认与 Script 版本设置、Agent 运行前输入选择、错误与恢复入口，以及其他节点创建方式。
- 删除 `ReplaceConnection` handler/service、GraphAPI 包装、Wails 生成绑定和旧来源替换相关状态/测试；移除仅由该命令调用的 `WorkspaceMover` 端口、实现、回滚逻辑与专属错误。`WorkspaceReader` 等共享工作区能力保留。
- 完整集成回归改为用户可执行的断开后新连，再验证新运行快照使用更新后的 Candidate；工作区视图测试验证不同 workspace root 之间不泄漏文件。

## 验证结果

- 聚焦 Go：`GOPROXY=off GOSUMDB=off go test ./internal/service/workflow ./internal/handler ./internal/app ./internal/domain/entity ./internal/infra/persistence ./internal/infra/workspace` 通过。
- Go 全量：`GOPROXY=off GOSUMDB=off go vet . ./internal/...` 与 `GOPROXY=off GOSUMDB=off go test . ./internal/...` 通过。
- 聚焦前端：7 个测试文件、96 项通过，覆盖画布关系栏缺席、快照更新后画布结构稳定、U23 拖线及线身断开/版本操作。
- 前端全量：24 个测试文件、241 项通过；`typecheck`、`check:tokens` 通过。`lint` 0 错误，保留 `usePendingController.tsx` 的 2 条既存警告。
- 绑定：`wails3 generate bindings -ts -i -clean=true` 成功，生成接口不再包含 `ReplaceConnection`。
- 前端生产构建通过；首屏 JS gzip 184.83 KB。保留 Vite 对单个 minified chunk 超过 500 KB 的既存提示。
- 离线 `wails3 build` 通过，生成 `bin/audio-ui.exe`；`bin/audio-ui.exe --version` 输出 `AudioUI v0.2.0`。构建时设置 `GOPROXY=off GOSUMDB=off npm_config_offline=true`，未下载依赖。

## AC 覆盖与未覆盖项

| AC | 自动化/构建证据 | 用户页面验收 |
|---|---|---|
| AC-U-63 | 已检查旧工具栏连接控件与来源替换操作不再渲染；新连/改接仅保留鼠标端口/箭头操作，其他键盘操作与关系浮层保留。 | 用户于 2026-11-15 确认验收通过。 |
| AC-U-64 | 选中 Agent、增加输入边并刷新快照时，测试确认不渲染输入关系栏，`.canvas-flow-root` 始终紧随工具栏；运行输入选择与错误提示组件保留。 | 用户于 2026-11-15 确认验收通过。 |
| AC-U-65 | Go 全量测试及集成回归通过；绑定、前端生产构建与离线 Wails 构建通过；旧运行/结果使用发送时快照的回归保留。 | 用户于 2026-11-15 确认验收通过。 |

用户于 2026-11-15 确认 AC-U-63～65 页面验收通过；页面操作步骤与截图未归档。助手未启动或操控浏览器/Wails 窗口，未执行真实 Agent、CLI 或网络访问。`bin/audio-ui.exe` 为本次当前工作树离线构建；自动化、构建与用户页面验收分别记录。
