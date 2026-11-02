# 版本管理规范

## 1. 版本号

采用语义化版本：`MAJOR.MINOR.PATCH`。

| 段 | 递增条件 |
|---|---|
| MAJOR | 不兼容的工程格式、协议或行为变更 |
| MINOR | 向后兼容的新功能 |
| PATCH | 向后兼容的缺陷修复、构建修复、非行为级 UI 调整 |

### 0.x 阶段策略

1.0.0 之前不承诺兼容边界：

- PATCH：修复、回归、构建问题。
- MINOR：用户可感知的新功能、行为变化、重要 UX 改进。
- MAJOR：暂不使用；1.0.0 留给首个稳定版。

### 里程碑映射

| 版本 | 里程碑 |
|---|---|
| 0.1.0 | M0 壳、绿色目录、日志 |
| 0.2.0 | M1 Agent 会话、MCP、权限 |
| 0.3.0 | M2 图、选题、提示词、待应用 |
| 0.4.0 | M3 场景 DSL、渲染预览 |
| 0.5.0 | M4 TTS、导出 |
| 0.6.0 | M5 队列、回收站、文档 |
| 1.0.0 | 首个稳定版 |

## 2. 预发布

格式：`X.Y.Z-alpha.N`、`-beta.N`、`-rc.N`。

- `N` 为非负整数，无多余前导零。
- alpha：内部验证。
- beta：功能冻结，供试用。
- rc：候选发布，只接受 blocker 修复。
- 不使用 `-1`、`-final` 这类语义不明的后缀。

## 3. 唯一来源

版本号的唯一来源是仓库根目录的 `VERSION` 文件。

构建时注入：

- `VERSION` 文件。
- `build/config.yml` 的 `info.version` 与 `frontend/package.json` 的 `version`。
- 构建产物元数据（如 `-X main.version=...`）。
- Git tag：`vX.Y.Z`（带 `v` 前缀）。

Wails v3 的 `wails3 build` 不转发 `-ldflags`：版本由根 `Taskfile.yml` 读取 `VERSION`，再在 `build/windows/Taskfile.yml` 的构建参数中注入 `-X main.version=...`。修改 `build/config.yml` 后先运行 `wails3 task common:update:build-assets`，再补 Taskfile，避免生成步骤覆盖。
- Release 标题：`AudioUI vX.Y.Z`。
- 产物名：`audio-ui-vX.Y.Z-win64.zip`。
- CHANGELOG 的 `## [X.Y.Z]`。
- 文档中的下载示例与版本引用。

任何一处不一致，拒绝发布。

## 4. 一致性门禁

发布前执行脚本检查：

1. `VERSION` == 预期版本。
2. CHANGELOG 存在对应版本段落。
3. 文档中的版本引用与 `VERSION` 一致。
4. tag 未创建或指向正确提交。
5. 产物名包含正确版本。

可自动化的项做成构建门禁；不可自动化的项写入检查清单。

## 5. 版本状态

| 状态 | 进入条件 |
|---|---|
| 规划中 | 已列入里程碑 |
| 开发中 | 分支已创建 |
| 冻结 | 范围与内容清单确认 |
| 已发布 | 产物与说明已归档 |
| 维护中 | 接受 PATCH |
| 停止维护 | 发布 EOL 公告 |

## 6. 分支与标签

- 主干：`main`。
- 发布分支：`release/x.y`（需要时）。
- 热修分支：`hotfix/x.y.z`，修复后回合主干。
- tag 使用 annotated tag，禁止移动已发布 tag。

## 7. 支持策略

- 当前稳定版：完整支持。
- 上一个 MINOR：发布后 30 天内接受 PATCH。
- 0.x：只维护最新 MINOR。
- 停止维护需在 CHANGELOG 与发布说明中公告。

## 8. 示例

```
VERSION=0.3.0
tag: v0.3.0
release title: AudioUI v0.3.0
artifact: audio-ui-v0.3.0-win64.zip
changelog: ## [0.3.0] - 2026-12-01
```
