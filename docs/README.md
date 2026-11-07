# 文档索引

> M2-11～13 已选择 RM-018 的本地脚本子范围；M2-11A 设计、M2-11B 工程格式/图领域模型与 M2-11C 发送快照均已验收；M2-11D 已验收（2026-11-07），完整产品闭环尚未实现。M2-00～10 旧模型验收事实保留；RM-019 云端与 RM-020 视频仍待评估。

| 文档 | 读者 | 状态 | 说明 |
|---|---|---|---|
| [product.md](product.md) | 产品 / 设计 / 开发 | 草案 | 产品总览：概念、流程、架构、数据模型 |
| [prd.md](prd.md) | 产品 / 开发 | 草案 | 需求、目标、范围、规则、非功能 |
| [architecture.md](architecture.md) | 开发 | 草案 | 技术方案、分层、数据流、存储 |
| [frontend-design.md](frontend-design.md) | 前端 / 设计 | 草案 | 设计令牌、组件、交互状态、响应式、可访问性、验收 |
| [design/style-directions.html](design/style-directions.html) | 设计 / 前端 | 原型 | 设计依据：主题、布局、交互 |
| [design/m0-startup-shell.html](design/m0-startup-shell.html) | 设计 / 前端 | 原型 | M0 启动自检壳（临时） |
| [design/m1-session-shell.html](design/m1-session-shell.html) | 设计 / 前端 | 原型 | M1 临时会话页与控制台（M2 已删除） |
| [design/m2-workspace.html](design/m2-workspace.html) | 设计 / 前端 | 原型 | M2 工作台：画布、草稿节点、候选端口、运行分组、待应用、运行路径锁、同父换候选确认 |
| [design/m2-09-canvas-demo.html](design/m2-09-canvas-demo.html) | 设计 / 前端 | 演示 | M2-09 节点画布、端口、草稿、改挂确认与视图控制本地演示 |
| [design/m2-10a-topic-run-demo.html](design/m2-10a-topic-run-demo.html) | 设计 / 前端 | 演示 | M2-10A 选题参数保存、一次性运行状态与结构化错误本地演示 |
| [design/m2-topic-closure-demo.html](design/m2-topic-closure-demo.html) | 设计 / 前端 | 演示 | 选题运行、待应用、候选分支及改挂的本地交互示例 |
| [design/target-agent-flow-demo.html](design/target-agent-flow-demo.html) | 产品 / 设计 / 开发 | 历史目标演示 | 较早的目标方向草案，含云端/视频示意；不代表当前 M2-11～13 范围 |
| [design/m2-11-13-local-flow-demo.html](design/m2-11-13-local-flow-demo.html) | 产品 / 设计 / 开发 | M2-11A 设计验收演示 | 本地候选→提示词→Agent→`script` 输出模拟交互；设计已验收，不是产品实现 |
| [design/canvas-edge-008-diagnosis.html](design/canvas-edge-008-diagnosis.html) | 开发 / 测试 | 诊断示意 | React Flow 动态候选端口、测量缓存与错误 008 的时序说明 |
| [design/canvas-drag-measurement-diagnosis.html](design/canvas-drag-measurement-diagnosis.html) | 开发 / 测试 | 诊断示意 | 拖动时节点测量状态丢失及修复思路的独立交互示意 |
| [dev/process.md](dev/process.md) | 开发 / 产品 | 草案 | 阶段、工作分解、门禁、变更控制 |
| [dev/roadmap.md](dev/roadmap.md) | 产品 / 开发 | 草案 | 需求优先级、批次、暂存需求池 |
| [dev/m2-workplan.md](dev/m2-workplan.md) | 开发 | 草案 | M2 逐任务执行手册、开工协议、任务卡与状态 |
| [dev/m2-target-closure-plan.md](dev/m2-target-closure-plan.md) | 开发 / 测试 | 草案 | M2-11～13 新节点模型与本地脚本闭环计划 |
| [dev/m2-topic-closure-plan.md](dev/m2-topic-closure-plan.md) | 开发 / 测试 | 计划 | M2 工作台至选题闭环的分项实现、测试与验收计划 |
| [dev/canvas-drag-rendering-fix-plan.md](dev/canvas-drag-rendering-fix-plan.md) | 开发 / 测试 | 草案 | 提示词节点拖动渲染错误的复现证据与修复计划 |
| [dev/m2-backend-closure-plan.md](dev/m2-backend-closure-plan.md) | 开发 | 草案 | M2-05～07 后端闭环实现计划与已确认决策 |
| [dev/evidence/m0.md](dev/evidence/m0.md) | 产品 / 测试 | 证据 | M0 自测与 GUI 冒烟记录 |
| [dev/evidence/m1.md](dev/evidence/m1.md) | 产品 / 测试 | 证据 | M1 会话/权限/resume 冒烟记录 |
| [dev/evidence/m2-03.md](dev/evidence/m2-03.md) | 产品 / 测试 | 证据 | M2-03 工作区与写作用域验收记录（用户已确认） |
| [dev/evidence/m2-04.md](dev/evidence/m2-04.md) | 产品 / 测试 | 证据 | M2-04 上下文构建器验收记录（用户已确认通过） |
| [dev/evidence/m2-05.md](dev/evidence/m2-05.md) | 产品 / 测试 | 证据 | M2-05 Pending 后端自检与验收记录（用户已确认通过） |
| [dev/evidence/m2-06.md](dev/evidence/m2-06.md) | 产品 / 测试 | 证据 | M2-06 MCP 只读图、结构化 reply 与 artifact 分块自检及验收记录 |
| [dev/evidence/m2-07.md](dev/evidence/m2-07.md) | 产品 / 测试 | 证据 | M2-07 运行管理与运行锁自检及验收记录 |
| [dev/evidence/m2-09.md](dev/evidence/m2-09.md) | 产品 / 测试 | 证据 | M2-09 画布与节点连接实现自检、门禁结果、真实窗口验收和 AC 映射（已验收） |
| [dev/evidence/m2-10a.md](dev/evidence/m2-10a.md) | 产品 / 测试 | 证据 | M2-10A 选题参数与一次性运行验收证据（已验收；M2-10B 已验收，M2-10C 已验收） |
| [dev/evidence/canvas-edge-008.md](dev/evidence/canvas-edge-008.md) | 开发 / 测试 | 证据 | 候选端口动态测量与派生边竞态修复自检、测试和浏览器截图 |
| [dev/evidence/canvas-drag-015-fix.md](dev/evidence/canvas-drag-015-fix.md) | 开发 / 测试 | 证据 | 节点拖动测量竞态修复、前端门禁、浏览器操作和 exe 资源核对 |
| [dev/evidence/agent-mcp-startup-race.md](dev/evidence/agent-mcp-startup-race.md) | 开发 / 测试 | 证据 | SDK 版本探测触发重复启动并清理活动 MCP 配置的原因、修复与验证 |
| [dev/evidence/m2-10b.md](dev/evidence/m2-10b.md) | 产品 / 测试 | 证据 | M2-10B 待应用与候选端口验收证据（已验收；浏览器实测留在 M2-10C） |
| [dev/evidence/m2-10c.md](dev/evidence/m2-10c.md) | 产品 / 测试 | 证据 | M2-10C 页面验收已由用户确认；附件截图未归档到仓库 |
| [dev/evidence/m2-11c.md](dev/evidence/m2-11c.md) | 产品 / 测试 | 证据 | M2-11C 发送快照、上下文与运行授权（用户已验收） |
| [dev/evidence/m2-11d.md](dev/evidence/m2-11d.md) | 产品 / 测试 | 自检与验收证据 | M2-11D 受限 reply、原子结果事务与恢复自检；不代表 M2-13 完整验收 |
| [agent-protocol.md](agent-protocol.md) | 开发 | 草案 | MCP 工具、能力位、权限、会话 |
| [acceptance.md](acceptance.md) | 产品 / 测试 | 草案 | 验收标准与发布门禁 |
| [decisions.md](decisions.md) | 开发 | 草案 | 架构决策记录 |
| [release/versioning.md](release/versioning.md) | 开发 / 发布 | 草案 | 版本号与一致性规范 |
| [release/release-process.md](release/release-process.md) | 开发 / 发布 | 草案 | 发布流程与角色 |
| [release/release-checklist.md](release/release-checklist.md) | 发布 | 草案 | 发布前/中/后检查项 |
| [release/release-notes-template.md](release/release-notes-template.md) | 发布 | 草案 | 发布说明模板 |

## 阅读顺序

1. 产品总览 → 理解概念与流程。
2. PRD → 确认目标、边界与验收。
3. 路线图 → 确认优先级、批次与暂缓项。
4. 架构 + Agent 协议 → 开发依据。
5. 开发流程 → 阶段、门禁与变更控制。
6. 验收标准 → 测试与发布门禁。
7. 发布规范 → 发版操作。

## 文档规则

- 所有文档以仓库内版本为准，不另存个人副本。
- 需求变更必须更新 PRD 的变更记录，并关联验收标准。
- 开发中暂无法处理的变更写入 [dev/roadmap.md](dev/roadmap.md) 暂存池，不直接扩大当前批次。
- 发版前检查 README、文档中的版本号、下载链接、命令示例是否同步。
- 草案文档标注状态；定稿后改为「已评审」。
