# 计划导航

> **状态以路线图为准**：[开发路线图](../dev/roadmap.md)。本目录按“当前待处理 / 阶段计划”分类，目录归类本身不代表计划已批准或纳入当前批次。

## 当前待处理

| 计划 | 状态 | 用途 |
|---|---|---|
| [画布、节点与 Agent 交互优化](active/canvas-workbench-optimization-plan.md) | U0 已确认；U1～U7、U10～U25 已按范围通过用户验收；U8 按离线核对范围完成；U9 fixture、自检、离线构建及用户最终页面验收完成；AC-U-18 未验收 | 包含多上游无环图、无 Topic 有效任务运行、版本引用及 Topic 新批次；U25 去除选中节点四角方块，已于 2026-11-15 通过用户验收。 |
| [提示词节点拖动渲染问题](active/canvas-drag-rendering-fix-plan.md) | 实现与本地自检完成，待评审 | 015 受控复现修复记录；Wails/WebView2 专项未覆盖。 |
| [前后端代码质量与精简函数注释](active/code-quality-optimization-plan.md) | 独立维护专项；Q-0～Q-2 已通过用户验收；Q-3～Q-7 未开始 | Q-0 恢复缺陷修正、Q-1 候选事务收口及 Q-2 图与上下文职责拆分见 `docs/dev/evidence/q0-pending-recovery-risk.md`、`docs/dev/evidence/q1-topic-candidate-transaction.md`、`docs/dev/evidence/q2-graph-context-responsibilities.md`。 |

## 阶段计划：M2

| 文档 | 状态与用途 |
|---|---|
| [M2 逐任务执行手册](stages/M2/m2-workplan.md) | M2 任务和验收状态索引；M2 整体状态以路线图为准。 |
| [M2-11～13 本地脚本闭环](stages/M2/m2-target-closure-plan.md) | 已验收阶段的范围、设计和证据索引。 |
| [M2-08～10 工作台与选题闭环](stages/M2/m2-topic-closure-plan.md) | 已完成子阶段的原始细化方案，保留历史决策与验收背景。 |
| [M2-05～07 后端闭环](stages/M2/m2-backend-closure-plan.md) | 已完成子阶段的原始实现方案，供追溯历史决策。 |

## 后续阶段

M3 尚未开始，当前没有独立的详细阶段计划；范围与出口见[路线图 M3](../dev/roadmap.md#44-m3-渲染)。进入 M3 规划时，再在 `stages/M3/` 建立任务文档。

旧阶段细化文档可能保留实施前的背景快照；**当前任务状态和范围只看路线图与 M2 逐任务执行手册**，已验收结果看 `docs/dev/evidence/`。

## 目录约定

- `active/`：尚待评审、实施或补充验收的计划。
- `stages/M2/`：M2 阶段总手册及已完成子阶段计划。
- `docs/dev/roadmap.md`：需求优先级、批次状态和暂存项的唯一状态源。
- `docs/dev/evidence/`：自检、验收记录和截图；证据不与计划混放。
