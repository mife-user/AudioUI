# 文档索引

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
| [dev/process.md](dev/process.md) | 开发 / 产品 | 草案 | 阶段、工作分解、门禁、变更控制 |
| [dev/roadmap.md](dev/roadmap.md) | 产品 / 开发 | 草案 | 需求优先级、批次、暂存需求池 |
| [dev/m2-workplan.md](dev/m2-workplan.md) | 开发 | 草案 | M2 逐任务执行手册、开工协议、任务卡与状态 |
| [dev/evidence/m0.md](dev/evidence/m0.md) | 产品 / 测试 | 证据 | M0 自测与 GUI 冒烟记录 |
| [dev/evidence/m1.md](dev/evidence/m1.md) | 产品 / 测试 | 证据 | M1 会话/权限/resume 冒烟记录 |
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
