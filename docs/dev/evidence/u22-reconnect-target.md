# U22 目标端原子重连自检记录

## 状态

后端命令、Handler、GraphAPI、生成绑定及 Go/前端自检与离线 Wails 构建已完成。用户于 2026-11-15 确认 U22 验收通过。U23 的箭头拖拽 UI 和 U24 的入口清理未实现；U22 具体验收步骤/截图未归档。

## 实现

- 新增工程作用域 `ReconnectTarget`，请求携带工程 ID、边 ID、原边完整身份（来源、目标、kind、Script 版本）和新目标。身份任一字段变化时返回 `EDGE_CONFLICT`，避免迟到意图修改其他边。
- 在 workflow 互斥锁内验证工程、普通边、目标、重复边、自连、成环和运行锁；同目标不写入，候选生成来源不可改接。
- 仅替换同一边的目标端，保留边 ID、来源、kind 和逐边 Script 版本（含 0）；一次保存并递增一次 revision。旧目标与新目标的后代合并去重后标记 stale，共享后代只纳入一次。
- 不调用 `WorkspaceMover`，不移动或删除工作区文件；已有 `workspaceRootId` 保持不变，未绑定节点沿用现有绑定规则。运行时若输出工作区身份不匹配，现有图快照校验明确拒绝。
- Handler 返回指向新目标的 mutation receipt；GraphAPI 暴露类型化请求，尚无前端交互调用。旧 `ReplaceConnection` 保留，本阶段不删除旧入口或旧命令。

## 自动化检查

工作目录：`D:\vscode\AI\bilibili\AudioUI`。测试均使用本地工程夹具，无真实 Agent、CLI、网络访问或用户工程数据。

| 检查 | 结果 |
|---|---|
| `go test ./internal/service/workflow ./internal/handler ./internal/app ./internal/domain/entity ./internal/infra/persistence` | 通过 |
| `GOPROXY=off GOSUMDB=off go vet . ./internal/...` | 通过 |
| `GOPROXY=off GOSUMDB=off go test . ./internal/...` | 全部通过 |
| `npm --prefix frontend run test` | 24 个测试文件、236 项通过 |
| `npm --prefix frontend run lint` | 0 错误；2 条既存 `react-refresh/only-export-components` 警告，位于 `usePendingController.tsx` |
| `npm --prefix frontend run check:tokens` | 通过 |
| `wails3 generate bindings -ts -i -clean=true` / `wails3 generate bindings -ts -i` | 首次显式清理生成调用被 Auto Mode 安全策略拦截；随后使用离线常规生成命令成功，`wails3 build` 中的绑定生成任务也成功。未手工编辑生成文件。 |
| `npm --prefix frontend run typecheck` | 通过；绑定生成后包含 `ReconnectTargetRequest` 与 `WorkflowService.ReconnectTarget`。 |
| `npm --prefix frontend run build` | 通过；Vite 提示单个压缩前 chunk 大于 500 kB，无构建错误。 |
| `wails3 build` | 通过；离线配置下完成绑定生成、前端构建及 Windows exe 构建。 |

## AC-U-55～59 自检映射

| 验收项 | 代码级覆盖与边界 |
|---|---|
| AC-U-55 | 检查同一边只改目标端、边 ID/来源/其他边不变、revision 仅递增一次；覆盖 Script 固定版本 0 与正数版本保留。 |
| AC-U-56 | 覆盖同目标无变化、自连、成环、重复来源-目标、缺失节点/边、候选来源保护、旧边身份冲突和工程切换范围；拒绝时不保存或发图变更。 |
| AC-U-57 | 覆盖来源、旧/新目标、两侧后代及共享后代锁；成功时合并去重旧/新影响范围并标记 stale。 |
| AC-U-58 | 覆盖旧边身份过期、工程范围错误、保存失败不产生半边；Handler receipt 及落盘结果通过重开工程检查。UI 迟到回执和“仅重试快照”属于 U23，未在本阶段实现或验收。 |
| AC-U-59 | 覆盖跨 Topic 根改接时工作区身份不变、不调用 mover，并验证身份不匹配的 Script 输出会被现有运行快照校验拒绝；Handler 测试重开工程确认拓扑持久化。未启动真实 UI。 |

## 用户验收

用户于 2026-11-15 确认 AC-U-55～59 验收通过；助手未启动或操控 UI。具体验收操作记录与截图未归档。

## 未覆盖与后续事项

- U22 门禁与验收均完成；依赖已满足，可按路线图推进 U23。
- 用户已完成 U22 验收；助手未启动或操控 UI。Windows/Wails 鼠标交互、DPI 和箭头拖拽属于 U23/U9，未在 U22 中实现或验收。
