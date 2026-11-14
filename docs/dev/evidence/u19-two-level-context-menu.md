# U19 两级画布右键菜单自检

## 状态

用户于 2026-11-14 确认 AC-U-49～50 验收通过。助手未启动或操控真实界面；具体操作记录和截图未提供，本记录不补写。

## 实现

- 画布空白处右键、Shift+F10／菜单键及叶子链快捷栏统一进入两级菜单：先显示「新增节点」，再显示节点类型。
- 选题、提示词、Agent、脚本为可创建项；Candidate 仅显示「由选题生成」，不可手动创建。
- 创建落点继续使用菜单触发时计算并冻结的画布坐标；菜单定位与避边不改变落点。
- 子菜单按可用空间向左或向右展开，并在垂直方向夹入视口；支持方向键、Home/End、Enter/Space、逐层 Escape 和焦点返回。
- 菜单选项沿用运行/图变更禁用态；创建失败显示错误并允许用户重新发起，进行中的创建会阻止再次选择。

## 自动化与构建

在仓库根目录 `D:\vscode\AI\bilibili\AudioUI` 执行：

| 检查 | 结果 |
|---|---|
| `npm --prefix frontend run test -- src/canvas/WorkspaceCanvas.test.tsx src/canvas/WorkspaceCanvas.realFlow.test.tsx src/canvas/ChainQuickBar.test.tsx` | 3 个测试文件、59 项通过 |
| `npm --prefix frontend run test` | 24 个测试文件、235 项通过 |
| `npm --prefix frontend run typecheck` | 通过 |
| `npm --prefix frontend run lint` | 0 错误；2 条既存 `react-refresh/only-export-components` 警告，位于 `usePendingController.tsx` |
| `npm --prefix frontend run check:tokens` | 通过 |
| `npm --prefix frontend run build` | 通过；首屏 JS gzip 183.45 kB。Vite 提示未压缩 JS chunk 600.22 kB，超过 500 kB 提示线 |
| `GOPROXY=off GOSUMDB=off npm_config_offline=true wails3 build` | 通过，生成 `bin/audio-ui.exe`；构建使用离线依赖配置，模块文件与依赖锁文件未产生改动 |
| `bin/audio-ui.exe --version` | 输出 `AudioUI v0.2.0` |

定向回归覆盖两级菜单显示与四种创建项、Candidate 限制、原始画布落点、Shift+F10／菜单键、方向键与逐层 Escape、快捷栏共用入口、四角避边、Prompt 输入框右键保留、创建失败后的重试及在途重复创建拦截。真实 React Flow 测试验证空白画布事件接线。

## 验收边界

自动化、类型检查、构建和 exe 版本核验不替代真实页面验收。用户于 2026-11-14 确认 AC-U-49～50 验收通过；具体逐项操作记录和截图未提供，本记录不补写。Wails/WebView2 页面由用户验收。讨论原型：[U17～U20 工作台示意](../../design/u17-u20-workbench-polish-demo.html)。
