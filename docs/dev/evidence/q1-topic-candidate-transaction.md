# Q-1 事务恢复与 Candidate 构造收口

| 项 | 结果 |
|---|---|
| 状态 | 用户验收通过（2026-11-16） |
| 关联 | 前后端代码质量专项 Q-1；AC-U-31～35、AC-U-43 |
| 范围 | Topic 首轮与重跑候选构造、事务恢复回归；未改变数据格式或协议 |

## 实现

- 首轮自动提交、普通待处理应用和重跑确认统一调用 `buildTopicCandidateBatch`。
- Candidate 的节点 ID、来源边 ID、WorkspaceRootID、round/artifact/index/runId 与自动避让位置在同一构造路径生成。
- `pending.txn.json` 写入、Artifact CAS、工程 revision、pending 索引和 journal 清理顺序未变。
- Q-0 冻结输出目标恢复修正未改动；已有 Script 基准版本冲突仍拒绝确认。

## 回归覆盖

- 首轮与 Topic 重跑使用相同 payload 时保留两个独立批次：CAS artifact 可相同，Candidate 来源 runId/round、节点与边 ID 独立，候选索引完整，节点不重叠。
- 同一 run 的首轮和重跑 reply 重放均返回原结果，不重复生成节点或递增 revision；不同 run 可复用同一 request ID。
- 普通 pending 应用校验 Candidate 来源身份；已有 Script 基准版本冲突不可由 force 绕过。
- Topic 重跑恢复覆盖“CAS 与 pending 索引已写、journal 尚未删除”的重放；Candidate 仍绑定本批 runId/round，即使 CAS 元数据来自先前同内容 artifact。
- 恢复校验拒绝被修改的 TopicBatch baseline 和跨 run Candidate 来源。已有提交各阶段失败、缺失索引重试、Script 追加/并行输出恢复用例一并回归。

## 验证

以下命令均设置 `GOPROXY=off GOSUMDB=off AUDIOUI_LIVE_CLI=0`，全部通过：

```bash
go test ./internal/service/workflow -run 'TestTopicFirstRunAndRerunKeepCandidateBatchIdentity|TestApplyTopicUsesRoundKindWhenArtifactMetadataWasFirstWrittenForAnotherKind|TestForceCannotBypassBoundScriptVersionConflict|TestSubmitRunReplyCommitFailuresKeepRecoveryState' -count=1
go test ./internal/infra/persistence -run 'TestCommittedTopicBatchReplyRecovery|TestCommittedTopicReplyRecoveryValidatesCandidateGraphAssociation|TestCommittedScriptReplyRecoveryValidatesVersionAssociation|TestCommittedRootlessDAGScriptReplyRecovery|TestCommittedCreateOutputDAGReplyRecovery|TestCommittedPendingRecoveryBlocksUntilIndexCanBeWritten' -count=1
go test . ./internal/...
go vet . ./internal/...
```

未运行前端门禁、绑定生成、Wails/GUI/浏览器或真实 Agent/CLI；本任务仅改 Go 服务与持久化测试，不涉及 UI、RPC、绑定或网络访问。