# 文档导航

> **先看这页，再按任务进入相应文档。** M2-11～13 本地脚本闭环已通过用户验收；独立工作台交互优化批次已确认并进行中；M3 尚未开始，状态以[路线图](dev/roadmap.md)为准。
>
> - [当前计划与阶段计划总览](plans/README.md)
> - [当前工作计划：画布、节点与 Agent 交互优化](plans/active/canvas-workbench-optimization-plan.md)
> - [M2 阶段任务手册](plans/stages/M2/m2-workplan.md)

## 按用途查找

| 你要查看 | 文档 |
|---|---|
| 当前阶段、优先级、暂缓项 | [开发路线图](dev/roadmap.md) |
| AI 视频业务与混合方向调研 | [RM-020 视频生产调研](dev/ai-video-workflow-research.md)（公开索引摘要，视频实现范围待评审） |
| M2 各任务状态与验收入口 | [M2 逐任务执行手册](plans/stages/M2/m2-workplan.md) |
| 当前待评审或待补验收计划 | [计划导航](plans/README.md) |
| 需求与范围 | [产品需求 PRD](prd.md)；[产品总览](product.md) |
| 架构、技术决策、Agent 权限 | [架构](architecture.md)；[ADR 决策记录](decisions.md)；[Agent 协议](agent-protocol.md) |
| 界面与交互规范 | [前端设计](frontend-design.md)；[设计方向原型](design/style-directions.html) |
| 验收条件 | [验收标准](acceptance.md) |
| 开发阶段与门禁 | [开发流程](dev/process.md) |
| 发布与版本 | [发布规范](release/)；[CHANGELOG](../CHANGELOG.md) |

## 阶段计划

阶段计划统一放在 [`plans/stages/`](plans/README.md) 下。当前保留 M2 主手册及各子阶段的原始方案，状态以路线图和 M2 手册为准；已完成计划保留用于追溯，不代表仍待实施。

## 设计原型与证据

- M2 本地脚本流程：[交互演示](design/m2-11-13-local-flow-demo.html)（模拟数据，不连接 Agent）。
- 画布整体优化：[交互原型](design/canvas-workbench-redesign-demo.html)（待评审方案，不代表产品已实现）。
- U10～U16 画布与重跑结果讨论：[单文件交互原型](design/u10-u16-canvas-optimization-demo.html)（模拟数据；多上游、无 Topic 运行原则及版本目标已确认，运行细节待评审）。
- U14 连线与运行过渡：[单文件交互演示](design/u14-dag-transition-demo.html)（已于 2026-11-12 通过用户验收；不连接工程或 Agent）。
- U11 Agent 收起小窗拖动：[单文件交互演示](design/u11-agent-rail-demo.html)（模拟数据，不连接 Agent、不访问工程或网络；不代表产品实现或验收证据）。
- 既有阶段自检与验收记录按编号存放在 [`dev/evidence/`](dev/evidence/m2.md)；M2-13 汇总见 [M2 集成证据](dev/evidence/m2.md)。
- 其它历史原型、诊断示意按主题存放于 `design/`，历史验收截图与记录存放于 `dev/evidence/`。

## 阅读顺序

1. [PRD](prd.md) 与 [路线图](dev/roadmap.md)：确认要做什么、当前做什么。
2. [计划导航](plans/README.md)：选择当前计划或对应阶段任务手册。
3. [架构](architecture.md)、[ADR](decisions.md)、[Agent 协议](agent-protocol.md)：确认技术与安全边界。
4. [前端设计](frontend-design.md) 与 [验收标准](acceptance.md)：确认交互要求及通过条件。
5. [开发流程](dev/process.md) 与[发布规范](release/)：确认门禁和交付流程。

## 文档维护约定

- 路线图是需求优先级、批次与暂存项的唯一状态源；M2 子任务细节由 M2 手册维护。
- 已完成的阶段计划保留历史决策和证据；当前工作以“计划导航”中的待处理计划为入口，避免在多个索引重复抄写状态。
- 新需求或范围变更需同步 PRD、验收标准和路线图；架构/协议/界面变化按项目规则更新对应文档。
- 草案须标注状态；只有用户评审后才标为「已评审」。
