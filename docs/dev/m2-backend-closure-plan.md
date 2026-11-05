# M2 后端闭环（M2-05～07）实现计划

| 项 | 内容 |
|---|---|
| 状态 | 计划（M2-05、M2-06、M2-07 已验收；M2-06/M2-07 于 2026-11-03 由用户确认） |
| 最后更新 | 2026-11-03 |
| 范围 | M2-05 Pending 后端、M2-06 MCP 只读与结构化回复、M2-07 运行管理 |
| 关联 | [M2 逐任务执行手册](m2-workplan.md)、[路线图](roadmap.md)、[开发流程](process.md)、[验收标准](../acceptance.md) |

> 本文用于保存用户确认的计划与决策；当前任务状态以 [M2 逐任务执行手册](m2-workplan.md) 为准。M2-05、M2-06、M2-07 已验收；M2-06 组件级实现曾作为 M2-07 前置继续推进。
>
> 这里的待应用、本机 MCP 文件工具和本地运行是既有 M2 基线。用户提出的新目标见 [PRD 1.1](../prd.md#11-新目标需求草案未实现) 与 ADR-027/028；不反向改写本计划及既有验收证据。

## 1. 背景与现状

M2-00～07 已由用户验收；M2-08～13 未开始。用户选择把关联最紧密的 M2-05 待应用后端、M2-06 MCP 只读与结构化回复、M2-07 运行管理作为一组规划。M2-06 组件级实现作为 M2-07 前置继续推进；GUI/live CLI 与端到端验收仍按后续任务范围覆盖。本文件只保存计划，当前状态以 M2 执行手册为准，不升版本。

现有基础：`internal/infra/persistence/{pending,artifacts,runs}.go` 已有存储；`internal/service/workflow` 已有图命令与运行路径锁；`internal/service/context/builder.go` 的 `Build`/`InputHashes` 已实现路径上下文与指纹；`internal/infra/agent/mcp/server.go` 仍是 M1 工具集；`internal/service/session` 和 `internal/app/agent.go` 仍使用 M1 会话。复用现有端口和组件，不重新实现它们。

## 2. 已确认决策与边界

- MCP 的图摘要、节点参数和产物只暴露当前运行路径；`get_graph` 不返回同树兄弟节点的 ID、标题或拓扑。同步修正现有协议中“整树摘要”的冲突表述。
- 对 `reply`：运行已结束，或目标是路径内但不是本次运行目标节点，返回 `TURN_NOT_ACTIVE`；访问路径外节点返回 `CHAIN_SCOPE_DENIED`。先检查运行有效性，再按作用域判断。
- 每 topic 树 50 条上限只计 `pending` 与 `invalid`；应用或丢弃后释放名额，历史与原文继续保留。按 pending 的运行路径快照确定所属树。
- M2-07 接入新运行服务后停用旧 M1 会话的执行和自动 resume，保留已迁移/历史数据。M2-07 阶段只提供后端调用和测试，节点执行入口到 M2-10/11、完整待应用界面到 M2-12；不得宣称届时已具备完整界面闭环。
- 不开放 Agent 写图/应用工具，不做 M2-08～13 的界面、M3 渲染、M5 队列/删除，也不改变 M2 全局单在途、一次性运行、路径隔离或节点锁定规则。Bash/同树兄弟文件访问的现有残余风险继续按 RM-017 明示，不声称 OS 沙箱。
- 运行看门狗：连续 60 分钟无有效进展返回 `RUN_TIMEOUT`，流式数据、工具开始/结束、权限/问题等真实运行事件重置计时；使用可注入时钟测试。权限/问题各自仍为 120 秒，超时不删除历史或已生成的 pending。

## 3. M2-05：Pending 与应用命令

**目标与验收**：F-07；覆盖 AC-M2-3/4、AC-07 的后端部分。未应用前图内容不变；应用后恰有一个不可变轮次、节点锁定、后代 stale。

1. 在 `internal/domain/{entity,port,valueobject}` 补齐按工程获取 pending store、状态更新、结构化校验、可选 `requestId` 持久化和上下文指纹计算等契约；沿用现有版本并兼容旧数据，不复制底层存储。`pending.json` 索引与原文分离，无效结果保留原文和校验错误。各 service 只依赖 domain 端口，由 app 注入 `context.Builder` 等实现，禁止 service 之间直接 import。
2. 在 `internal/service/workflow/pending.go` 内实现创建/列出/详情/应用/丢弃，复用 workflow 互斥与 `entity.Graph` 的路径/子树能力；与 `project.New/Open` 共用工程级操作协调，避免工程切换穿插多文件事务，并明确锁顺序以避免死锁。锁内原子校验每运行至多一个结构化结果、按运行+`request_id` 幂等、每树待处理上限及 256 KiB 限额。M2-06 的 MCP 通过端口调用创建；handler 只提供 UI 可用的查询/应用/丢弃，不暴露给 MCP。
3. 共用一份 `topics`/`script` 校验：topic 3～10 个候选、prompt 至少一个场景，检查协议所列必需字段与类型；`scene_dsl` 拒绝。保留额外字段，**不擅自加 `duration > 0` 等限制**，因为现有构建器示例使用 `duration: 0`。无效结果标记 `invalid` 且不可应用；只校验入口不写图。
4. 应用时在 workflow 命令锁内重取图，检查节点、类型、pending 状态、运行锁及内容锁；用 `context.Builder.InputHashes` 重算 `ancestorHash`/`paramsHash`，变化分别返回 `CONTEXT_CHANGED`/`PARAMS_CHANGED`；`force` 仅覆盖这些指纹冲突，不能绕过无效 Schema、丢失节点、运行锁或 `NODE_LOCKED`。对有效 JSON 用保留数字精度的确定性规范化字节生成 SHA-256 CAS，保留 pending 原始字节；追加唯一轮次、锁定内容与执行、后代标 stale，revision 更新后发布变更。丢弃幂等且不删除原文；已应用后丢弃明确报错。
5. 为跨 `pending.json`、`project.json` 的创建/应用/丢弃及 revision 更新增加工程内恢复日志：先原子持久化意图和必需原文/CAS，再以保存后的 `project.json` revision 为提交点，最后原子更新 pending 索引并清理日志。相关读写与工程切换共享事务协调，提交尚未完成或恢复失败时不向调用方暴露半成品状态；注意现有 `ProjectSession.Save` 会更新内存副本，需一并处理。工程启动/打开暴露前按提交点幂等恢复；提交前失败保持旧状态，提交后索引失败保留日志并阻止继续操作，修复后再对外报告状态；不删除可能被共享的 CAS，允许留下安全的孤立产物供后续回收。对损坏或无法判定的日志显式报错，不静默重建。
6. `ArtifactStore.Put` 现按 payload 哈希去重，meta 取首次写入；不能把首写 `meta.kind/nodeId` 当作后续轮次归属。核对并修正 `internal/service/context/builder.go`、`internal/service/workflow/candidates.go` 等消费者：授权以当前轮次为准，对跨 kind 复用核验 payload 符合相应 Schema，不放宽已存在的类型错误检查；旧轮次和 M2-04 已验收 fixture 不因新增 Schema 被追溯拒绝。保持既有 hash 与候选隔离规则。

**主要文件**：`internal/service/workflow/{pending,service,candidates,errors}.go`、`internal/service/context/builder.go`、`internal/domain/{entity,port,valueobject}`、`internal/infra/persistence/{pending,artifacts,project,atomic}.go`、`internal/service/project/service.go`、`internal/handler/workflow.go`、`internal/app/{project,workflow}.go` 及对应测试/绑定。

**专项验证**：合法/无效原文、字段/数量/大小边界、同运行并发与幂等、50 条与释放、各类冲突和 force、运行锁/内容锁、重复应用及丢弃、子树 stale、规范化去重、同 payload 跨 kind；每个写入步骤注入失败与重启恢复，验证图、索引、artifact、revision 一致且不重复追加。此阶段用可信的模拟运行快照，不提前实现 MCP/真实运行。

## 4. M2-06：MCP 只读图与 reply

**目标与验收**：F-04/F-07；覆盖 AC-M2-3、AC-M2-12 的工具部分。依赖 M2-05 用户验收通过；实际运行装配留 M2-07。

1. 扩展 `internal/domain/port/agent.go` 及必要的图读取/结果写入端口，以 runId/treeId/nodeId/chainPath + 生命周期绑定工具服务。保留 M1 测试所需隔离能力，不让 MCP 直接依赖 infra 或直接修改图。
2. 扩展 `internal/infra/agent/mcp/server.go`、`internal/app/tools.go`：注册 `get_chain/get_graph/get_node/list_node_types/validate/read_artifact/reply`，复用已存在的叠加文件工具和权限工厂。`get_graph/get_node` 只给当前路径所需摘要和元数据；`validate` 调用 M2-05 唯一校验器且不落盘；`reply(message)` 进 transcript，结构化 reply 委托 M2-05 生成 pending。`reply` 用 MCP SDK 的原始 arguments 提取 `payload` 字节，并让输入 schema 允许内容进入业务校验，避免 SDK 预先拒绝不符合业务 Schema 的有效 JSON 导致无效结果无法落盘；传输层 JSON 本身不合法时仍返回协议错误。空/无效输入、节点类型不符、过期运行、重复提交及数量限制按协议返回明确结果。
3. `read_artifact` 先按运行路径祖先轮次授权，再按轮次种类读取；topic 必须先从原始产物过滤出路径选中的单个候选，绝不回退完整内容。对过滤后紧凑 JSON 的 UTF-8 字节分页，每次最多 16 KiB；返回实际下一偏移，对非字符边界偏移或过小 limit 明确报错，避免把半个 UTF-8 字符伪装成完整文本。相同 ID 对应多个祖先且视图冲突时 fail closed，不能因首写 meta 或重复 ID 绕过过滤。
4. 新工具纳入 `internal/service/permission/policy.go` 自动允许范围，但服务端继续强制作用域；绝不注册 `create_node/connect/update_params/apply_artifact`。每运行的 MCP token/权限/生命周期在本阶段用模拟作用域覆盖，真实接线在 M2-07。

**主要文件**：`internal/domain/port/agent.go`、`internal/service/workflow/` 的只读适配与 M2-05 校验器、`internal/infra/agent/mcp/{server,tools_test,server_test}.go`、`internal/service/permission/policy.go`、`internal/app/tools.go`。

**专项验证**：完整工具清单和禁止工具；本路径/兄弟/跨树/目标错误与失效 token；topics/script/schema/invalid/大小上限/同 request_id、并发 reply 与运行结束竞态；topics 不同候选及重复 artifact ID 无泄露；UTF-8 多字节边界、空块、非法 offset/limit、分块重组、16 KiB 上限；文件叠加、ask_user 和 M1 现有自动化回归。

## 5. M2-07：运行生命周期和运行级权限

**目标与验收**：F-06/F-13；覆盖 AC-06、AC-13、AC-M1-1/2/5、AC-M2-8/9/10/11 的后端部分。依赖 M2-03/04/06 验收通过。

1. 在 `internal/service/run/` 增加 run manager 与拥有 runId 的全局单槽 gate。获取 gate 后，在 workflow 的同一互斥临界区完成执行资格校验、真实路径快照和路径锁获取，不分开调用 `EnsureExecutable`/`AcquireRunLock` 造成竞态；协调 `project.New/Open` 与运行启动，运行期间禁止切换工程。
2. 用锁定快照创建 `workspace.NewRunView`/对应 `RunGuard`、读取配置预算、调用 `context.Builder.Build`；输出首次消息和三个哈希。上下文过大或任一步启动失败都显式报错，不能启动半成品运行；创建每运行独立 MCP/token/权限控制器和一次性 driver，按运行注入工具和 system prompt，不调用 `Resume`。
3. 运行状态按 `starting → running → succeeded/failed/interrupted` 落入既有 `RunStore`；实现停止、退出、错误与超时的唯一终结路径。无论进程启动失败、发送失败、流结束、权限等待、中断或取消，均取消在途请求、关闭 CLI/MCP、清除临时配置、释放路径锁及 gate；清理使用独立于已取消请求的上下文并保证幂等。运行结束后 token 失效；仅运行期间所需的 `data/agent/<projectId>/<runId>/mcp.json` 临时含 token，禁止写入日志/运行历史，异常退出后下次启动清理残留。
4. 每个运行独立注入 `permission.Service` 和 `RunGuard`，扩展审计记录携带 runId/treeId/nodeId/chainId；保留允许决策审计失败即拒绝、权限/问题 120 秒规则及 RM-017 风险边界。
5. 扩展 `internal/infra/agent/claudecli/driver.go` 与 domain 事件：解析流、工具、权限、问题、完成/错误与 usage；每运行独立 seq、带身份与路径快照；复用 `internal/app/eventbus.go` 发布，运行记录 transcript 上限 500。使用 SDK 首个主运行 `AssistantMessage.Usage` 计算首次请求输入 token（含 cache read/creation），避免把多轮累计值误当上下文大小；缺失则显式未知，不伪造 0；`ResultMessage.Usage` 可作为独立累计值，复用 `ContextEstimate.WithActualInputTokens` 比较预算。测试多事件重复 usage 不重复计数。
6. 启动/打开每个工程时将持久化 `starting/running` 标为 `interrupted`，不自动重跑；保留旧 M1 历史与迁移备份。调整 `internal/app/{agent,workflow,app,eventbus}.go` 的组装与关闭路径；新增或扩展 run handler，提供按 node 启动、按 runId 查询/列表/停止/回答权限和问题的后端接口，绑定目标与作用域，不增加自由提示词输入。停用旧 M1 执行；新运行通过后端接口验证。M2-08 删除旧前端前，暂保留旧 `AgentService` 的绑定兼容外壳，读取历史仍可用，旧发送/新会话明确返回停用错误且不启动或 resume 进程，保证现有前端可编译、应用可启动；不新增界面。发布携带 runId 与路径快照的 `run:<type>` 事件；starting/running/terminal status 事件也作为运行路径锁变更信号，前端接入留 M2-08。

**主要文件**：`internal/service/run/`、`internal/domain/{port,entity,valueobject}`、`internal/infra/agent/{claudecli,mcp}`、`internal/infra/persistence/runs.go`、`internal/service/{workflow,project,permission}`、`internal/infra/audit/logger.go`、`internal/handler/`、`internal/app/` 及对应测试。

**专项验证**：启动/完整运行、每终态 gate 释放、并发 BUSY、共享祖先与兄弟分支锁、工程切换竞态、重启中断/历史、runId 和 seq、transcript 截断、真实 usage 与未知值、token/配置清理、120 秒权限与问答、审计失败 fail closed、旧 M1 不再执行；注入时钟验证 60 分钟无进展触发 `RUN_TIMEOUT`、有效进展重置及超时后资源释放。用 fake driver/MCP + 临时工程做 topic→pending→应用→候选分支→prompt 的后端集成测试。live CLI 仅在用户另行明确同意外部访问和潜在费用后运行，使用合成数据、只执行对应 M2 用例并记录结果；未经同意标记未执行。

## 6. 文档、门禁与交付

- 分任务同步权威文档并在对应阶段实现前记录技术方案：M2-05 更新 `docs/decisions.md` 的 ADR-022、`docs/architecture.md` 的存储/恢复、`docs/agent-protocol.md` 的 Schema 与限额；M2-06 更新 ADR-023、`docs/agent-protocol.md` 的作用域/错误码/分块协议及 `docs/acceptance.md`；M2-07 更新 ADR-021/024/026、架构、协议、验收、`CLAUDE.md` 及必要的 README。`docs/prd.md`、`docs/product.md` 等按受影响行为同步，不改已有需求而不留决策记录。
- 每项更新 `docs/dev/m2-workplan.md` 对应任务卡、`docs/dev/roadmap.md` 的状态描述、`CHANGELOG.md` Unreleased，创建 `docs/dev/evidence/m2-05.md`、`m2-06.md`、`m2-07.md` 并在 `docs/README.md` 索引；仅将自检完成项记为“待验收”，用户确认后才记“已完成”。不得改变后续任务状态，也不把局部测试写成完整 AC 通过。
- 每阶段执行工作手册列出的任务级门禁：`gofmt`、`go vet . ./internal/...`、`go test . ./internal/...`、绑定生成、前端 `typecheck/lint/check:tokens/test/build`、`wails3 build`，另跑上述专项测试；避免 `go test ./...`。检查 `git status`，不提交、不推送。
- 当前工作区存在未提交及未跟踪的 M2-03/04 变更；实施前读取并保留，不覆盖或重置。涉及生成物/构建的命令先检查目标，必要时在隔离副本中运行并报告，避免覆盖现有数据、绑定、dist 或 exe。
- 本计划不宣称已运行测试。阶段性交付包含具体命令、结果、失败与跳过项、AC 映射、证据路径及剩余风险，交用户逐项验收。
