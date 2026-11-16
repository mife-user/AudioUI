# 文档导航

> **先看这页，再按任务进入相应文档。** M2 已于 2026-11-08 通过用户整体验收，M2-11～13 本地脚本闭环亦已验收；当前进行的是 M2 后、M3 前的独立工作台交互优化批次。U16、U17 已通过用户验收，U18 已于 2026-11-14 通过用户验收，U19 已于 2026-11-14 通过用户验收；U20 已于 2026-11-14 通过用户验收；U21 已于 2026-11-14 通过用户验收；U22 已于 2026-11-15 通过用户验收，绑定生成和离线构建门禁通过；U23 已于 2026-11-15 通过 AC-U-60～62 页面验收；U24 已于 2026-11-15 通过 AC-U-63～65 页面验收；U25 已于 2026-11-15 通过 AC-U-66～67 页面验收；U8 已按离线能力核对范围完成（WebSearch 未接入，AC-U-18 未验收）；U9 fixture、代码自检、离线构建门禁及用户最终页面验收已完成；AC-U-18 未验收；M3 尚未开始，状态以[路线图](dev/roadmap.md)为准。
>
> - [当前计划与阶段计划总览](plans/README.md)
> - [当前工作计划：画布、节点与 Agent 交互优化](plans/active/canvas-workbench-optimization-plan.md)
> - [已评审代码质量计划：前后端优化与精简函数注释](plans/active/code-quality-optimization-plan.md)（独立维护专项及 M3 启动前置；Q-0、Q-1 已通过用户验收；Q-2～Q-7 未开始）
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
- U10～U16 画布与重跑结果讨论：[单文件交互原型](design/u10-u16-canvas-optimization-demo.html)（模拟数据；U15 已通过用户界面验收）。
- U16 结果版本与 Topic 批次：[单文件交互原型](design/u16-output-version-demo.html)（模拟数据，不连接工程或 Agent；已通过用户验收，原型不代表实现或验收证据）。
- U14 连线与运行过渡：[单文件交互演示](design/u14-dag-transition-demo.html)（已于 2026-11-12 通过用户验收；不连接工程或 Agent）。
- U11 Agent 收起小窗拖动：[单文件交互演示](design/u11-agent-rail-demo.html)（模拟数据，不连接 Agent、不访问工程或网络；不代表产品实现或验收证据）。
- U17～U20 后续界面优化：[单文件交互示意](design/u17-u20-workbench-polish-demo.html)（设计已评审；U17 已通过用户验收，U18 已于 2026-11-14 通过用户验收，U19 已于 2026-11-14 通过用户验收；U20 已于 2026-11-14 通过用户验收。模拟数据，不连接 Agent、不访问工程或网络，不代表产品实现或验收证据）。
- U21～U24 链路收纳与目标端拖拽：[单文件交互示意](design/u21-u24-chain-reconnect-demo.html)（U21 已于 2026-11-14 通过用户验收；U22 已于 2026-11-15 通过用户验收；U23 已于 2026-11-15 通过用户页面验收；U24 已于 2026-11-15 通过 AC-U-63～65 页面验收；模拟数据，不访问工程、Agent 或网络，不代表实现或验收证据）。
- U25 选中节点四角外观：[调整前后单文件示意](design/u25-selected-node-outline-demo.html)（用户于 2026-11-15 通过 AC-U-66～67 页面验收；静态模拟，不连接工程或网络、不模拟缩放，不代表实现或验收证据）。
- U25 实现与门禁：[自检及离线构建记录](dev/evidence/u25-selected-node-outline.md)（自动化检查、离线 exe 构建及用户页面验收已通过）。
- U8 本地 Agent 搜索能力：[离线核对记录](dev/evidence/u8-agent-search-capability.md)（用户确认按离线核对范围完成；未接入 WebSearch，AC-U-18 未验收，未访问外网）。
- U9 最终集成：[fixture 与离线自检证据](dev/evidence/u9-integration-fixture.md)（用户已确认最终页面验收通过；AC-U-18 未验收）。
- U22 目标端原子重连：用户于 2026-11-15 验收通过；命令与自检见 [U22 证据](dev/evidence/u22-reconnect-target.md)。
- U23 箭头拖拽与关系浮层：[自检与页面验收证据](dev/evidence/u23-edge-reconnect.md)（用户于 2026-11-15 确认 AC-U-60～62 验收通过）。
- U24 清理旧连接入口与关系栏：[实现、自检及页面验收记录](dev/evidence/u24-canvas-relation-cleanup.md)（用户于 2026-11-15 确认 AC-U-63～65 验收通过）。
- 前后端代码质量专项 Q-0：[事务恢复风险核对](dev/evidence/q0-pending-recovery-risk.md)（多 Script 输出恢复缺陷已复现并修正，用户已验收）。
- 前后端代码质量专项 Q-1：[事务恢复与 Candidate 构造收口](dev/evidence/q1-topic-candidate-transaction.md)（用户于 2026-11-16 验收通过）。
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
