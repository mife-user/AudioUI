# U14 多上游无环连接自检

## 实现范围

- `project.json` 升为 v4，`Project.Edges` 以稳定边 ID 持久化 `connection` 与受保护的 `candidate_source` 两类有向边；节点不再以 `parent` 作为拓扑真相。v3 及更早开发工程不迁移、不能直接打开，但不删除文件。
- Go 命令层新增 `Connect(source, target)`、`Disconnect(edgeId)`、`ReplaceConnection(edgeId, newSource)`；创建、连接、断开与改挂在同一工程事务中校验端点、自连、任意长度环、重复边、运行锁与工作区身份，失败不留下半条关系。
- 普通连接保留目标全部上游；重复连接幂等。候选来源由结果事务原子生成，普通连线不能修改来源身份，断开/改挂来源边返回 `PROTECTED_EDGE`。
- 节点记录 `workspaceRootId`：首次形成可确定的旧契约单路径时绑定，新增边与断开不移动文件；身份与当前路径不一致时阻止运行。仅显式改挂在影响范围均为单根且无共享后代时迁移 steps 并同步身份，否则返回 `WORKSPACE_MIGRATION_UNSAFE`。
- 运行过渡：旧有效 Topic 根单路径仍冻结既有快照并运行；无 Topic、路径存在其他输入、连接组合不符合旧执行契约或多个 Script 输出时，启动前返回 `RUN_GRAPH_UNSUPPORTED` 或 `OUTPUT_BINDING_CONFLICT`，不删除连接、不启动上游。
- 工作台快照新增 `edges`、`nodePaths`、`chain.nodeIds/ambiguous`；画布按快照绘制全部边，五类节点均提供手动输入/输出端口，键盘可选择目标新增连接，并在输入关系列表中明确改挂或断开；候选生成来源只读。汇合叶子显示真实上游集合，不伪造单一路径。
- 交互方案与讨论演示：`docs/design/u14-dag-transition-demo.html`（单文件模拟数据，已用 jsdom 自检新增、去重、改挂、取消、断开、成环拒绝和运行提示；未进行真实页面验收）。

## 自动化测试与构建

在 `AudioUI/` 执行，使用已有本地依赖并保持离线：

| 检查 | 结果 |
|---|---|
| `gofmt -w main.go internal` | 已执行；同时规范化了少量既有文件的 gofmt 对齐 |
| `go vet . ./internal/...` | 通过 |
| `go test . ./internal/...` | 全部通过 |
| `wails3 generate bindings -ts -i -clean=true` | 通过 |
| `npm --prefix frontend run typecheck` | 通过 |
| `npm --prefix frontend run lint` | 0 错误；`usePendingController.tsx:54,248` 有 2 条既有 Fast Refresh 警告 |
| `npm --prefix frontend run check:tokens` | 通过 |
| `npm --prefix frontend run build` | 通过；Vite 提示 JS chunk 约 587 kB，超过 500 kB 提示阈值 |
| `GOPROXY=off GOSUMDB=off npm_config_offline=true wails3 build` | 成功，生成 `bin/audio-ui.exe` |
| 前端定向测试 | `flowModel` 14 项、`realFlow` 26 项、其余 6 个轻型文件 36 项、`WorkspacePage` 5 项，共 81 项通过 |
| `CanvasNodes.test.tsx` 单文件 | 运行超过 150 秒无输出后停止，不计为通过 |
| `WorkspaceCanvas.test.tsx` 单文件 | 既有记录为多次无输出；本次未再计入通过 |

新增 Go 覆盖包括：v4 任意类型与同类型连接、多上游保留、长环/自连/重复/受保护来源拒绝、改挂迁移与回滚、工作区身份不匹配阻断、多个 Script 输出运行歧义、工程保存重开边关系、结果提交与恢复校验边和冻结输出绑定。

## 验收边界

- 未启动浏览器、桌面窗口或真实 Agent，未下载依赖或访问外网；页面与交互由用户验收。
- `CanvasNodes.test.tsx` 与 `WorkspaceCanvas.test.tsx` 在本机仍无法完成单文件运行，不能视为完整前端测试通过；已通过的定向文件覆盖画布模型、真实 React Flow DOM 和页面级选择流程。
- 真实 Windows/WebView2 输入法、DPI、真实 Agent 运行与旧开发工程升级不在本次范围。
