# Q-2 图与上下文职责拆分自检记录

| 项 | 结果 |
|---|---|
| 状态 | 用户于 2026-11-16 验收通过 |
| 关联 | 前后端代码质量专项 Q-2；F-03/F-17；AC-U-36～43 回归 |
| 范围 | 图校验与运行快照、运行快照验证、上下文解析职责拆分；补充 v5 DAG 多边版本回归 |

## 实现

- 将快照入口集中在 `graph_snapshot.go`，legacy 与单路径快照移至 `graph_snapshot_path.go`，多输入 DAG 快照移至 `graph_snapshot_dag.go`；图校验与 legacy forest 校验移至 `graph_validation.go`。
- 将 `RunExecutionSnapshot` 的校验与输入角色映射移至 `run_snapshot_validation.go`，与运行记录和快照数据结构分离。
- 将 legacy、Topic 路径、DAG 输入解析及快照产物读取移至 `internal/service/context/builder_resolve.go`；Builder 保留构造、上下文呈现与哈希 API，预算降级循环移至 `builder_budget.go`，入口继续负责预算参数校验和结果组装。
- 新增 v5 DAG 回归：同一 Script 的多条来源边分别固定 v1/v2；相同 v1 只读取一次但仍保留各自来源边；验证无 Topic、多 Prompt 顺序、未选输入隔离、预算降级和超限错误。
- 未更改快照/上下文字段、JSON、错误码、hash/golden、预算策略、图权限或工作区范围；未改前端、RPC、工程格式或 Agent 行为。

## 验证

以下命令均离线执行并通过：

```bash
GOPROXY=off GOSUMDB=off AUDIOUI_LIVE_CLI=0 go test ./internal/domain/entity ./internal/service/context -count=1
GOPROXY=off GOSUMDB=off AUDIOUI_LIVE_CLI=0 go test . ./internal/...
GOPROXY=off GOSUMDB=off go vet . ./internal/...
git diff --check
```

`gofmt` 已运行于本次修改的 Go 源文件和 `builder_test.go`。未运行前端门禁、构建、GUI/浏览器、真实 Agent/CLI 或网络测试；本任务不涉及 UI 或外部访问。

## 未覆盖与验收边界

本任务是职责整理与自动化回归，没有新增产品行为。自动化通过不替代用户对代码差异和专项出口的评审；Q-2 已于 2026-11-16 通过用户验收，Q-3～Q-7 未开始，M3 仍受代码质量专项总出口门禁约束。
