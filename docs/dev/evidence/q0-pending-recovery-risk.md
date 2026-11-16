# Q-0 基线与事务恢复风险核对

| 项 | 结果 |
|---|---|
| 状态 | 已通过用户验收（2026-11-16） |
| 范围 | 验证并修正 v5 DAG 中 Agent 多个 Script 输出时的已提交结果恢复；未进入 Q-1 Candidate 构造整理 |
| 日期 | 2026-11-16 |
| 缺陷分级 | major：特定事务崩溃窗口中，合法已提交工程无法加载；事务日志保留，数据不会被该恢复失败覆盖。 |

## 核对结果

确认 `validateAppliedReplyGraph` 原先遍历 Agent 的所有 Script 子节点，并在遇到第二个节点时拒绝恢复；该检查发生在使用冻结输出绑定之前。v5 DAG 允许多个 Script 输出，而运行快照会冻结本次选定目标，因此合法的版本追加或并行新建结果都可能在工程已提交后被恢复流程拒绝，导致工程无法正常加载。

新增并行输出恢复场景后，修复前两个定向用例均可复现：

```text
TestCommittedRootlessDAGScriptReplyRecovery
PENDING_TRANSACTION_CONFLICT: persisted result does not match pending: Agent has multiple Script outputs

TestCommittedCreateOutputDAGReplyRecovery
PENDING_TRANSACTION_CONFLICT: persisted result does not match pending: Agent has multiple Script outputs
```

修正后，已绑定结果按 `OutputBinding.NodeID` 从 Agent 的直接 Script 子节点中定位；绑定目标缺失、非直接子节点、版本关联不匹配等情况仍由现有校验拒绝。未绑定的旧结果仍要求唯一 Script 输出。

## 恢复窗口覆盖

| 崩溃窗口 | 覆盖 | 结果 |
|---|---|---|
| 工程 revision 尚未提交 | `TestProjectLoadDiscardsUncommittedPendingTransaction` | 以 base revision 重开时丢弃事务；不生成 pending 索引，工程 revision 保持不变。 |
| 工程 revision 已提交、事务尚未恢复 | `TestCommittedRootlessDAGScriptReplyRecovery` 与 `TestCommittedCreateOutputDAGReplyRecovery` | 重开后按冻结目标恢复；选定输出追加到 v2，另一并行输出仍保持 v1。 |
| artifact 与 pending 索引已写、事务日志尚未删除 | `TestCommittedCreateOutputDAGReplyRecovery` | 预先写入 artifact 和 pending 索引后重开，恢复幂等完成并删除事务日志；工程中既有输出与新建并行输出均保留。 |

## 验证

- 修复前定向复现：
  `GOPROXY=off GOSUMDB=off go test ./internal/infra/persistence -run 'TestCommittedRootlessDAGScriptReplyRecovery|TestCommittedCreateOutputDAGReplyRecovery' -count=1` — 两项按预期失败，错误均为 `PENDING_TRANSACTION_CONFLICT` / `Agent has multiple Script outputs`。
- 修复后定向验证：同一命令通过。
- Go 静态检查：`GOPROXY=off GOSUMDB=off go vet . ./internal/...` — 通过。
- Go 全量测试：`GOPROXY=off GOSUMDB=off AUDIOUI_LIVE_CLI=0 go test . ./internal/...` — 通过。
- 差异空白检查：`git diff --check` — 通过；Git 提示两处 Go 文件下次触及时 LF 可能转换为 CRLF，不影响检查结果。

本项只改后端恢复校验及持久化测试。未运行前端门禁、绑定生成、Wails 构建、真实 Agent/CLI、GUI/浏览器或网络测试；这些不属于 Q-0 的 Go 恢复核对范围。
