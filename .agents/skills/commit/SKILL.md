---
name: commit
description: 本仓库的 Git 提交信息规范（Conventional Commits 风格，含类型与固定图标约定）
whenToUse: 当用户让我生成提交信息、提交代码、总结变更或整理 git 历史时
---

# Git 提交规范

本仓库采用 **Conventional Commits** 风格的提交信息，并为每种类型指定了固定图标。

## 提交前必须经用户交互式确认（强制）

**任何 `git commit` 之前都必须由用户确认，且必须是交互式的**：先把准备提交的内容展示出来，停下来等用户回复，得到明确同意后才能执行提交。

- 禁止在用户未回复确认时自行提交；用户没有回答不等于同意，也不要拿"之前提过要提交"当作授权。
- **逐次确认**：上一条提交被批准，不构成对下一条提交的授权；每次提交都要重新展示并确认。
- 展示内容至少包含：
  - 将要提交的文件清单（`git status --porcelain` 的输出），并说明哪些改动**不会**进入本次提交（未跟踪、被忽略或故意留下的文件）。
  - 完整的提交信息（标题行 + 正文要点），可直接复制。
  - 涉及多种类型改动时，给出拆分提交的建议。
- 顺序：`git add` / `git status` 准备好 → 展示并等待确认 → 用户同意后 `git commit`。
- 用户要求调整提交信息或提交范围后，改完要重新展示并再次确认。
- 未经确认前不要执行 `git commit`、`git commit --amend`、`git push` 等任何写入历史的操作。

## 提交格式

```
<icon> <type>(<scope>): <subject>
```

- `icon`: 提交类型图标（见下表）
- `type`：提交类型（见下表）
- `scope`（可选）：影响范围，如 `core`、`web`、`auth`、`user` 等
- `subject`：简短描述，使用中文或英文均可，但同一仓库建议统一

## 提交正文（Body）

非 trivial 的提交必须带正文：标题行后空一行，用 `- ` 无序列表逐条说明改动要点（每条一句话，说清"做了什么"而非复述标题）。琐碎改动（一两个文件的小修复）可只写标题。

模板：

```
<icon> <type>(<scope>): <subject>

- 改动要点 1
- 改动要点 2
- 改动要点 3
```

示例：

```
✨ feat(user): 新增用户导出功能与列表搜索优化

- 新增用户列表 Excel 导出接口，支持按搜索条件过滤
- 列表查询支持按部门与状态组合筛选
- 修复导出大数据量时的内存溢出问题
- 同步生成 gfa-web 前端 API 客户端代码
- 补充导出功能的单元测试
```

## 提交类型与图标

| 类型       | 图标 | 描述                                                        |
|------------|------|-------------------------------------------------------------|
| `init`     | 🎉   | 初始化项目                                                  |
| `feat`     | ✨   | 新功能                                                      |
| `fix`      | 🐞   | 错误修复                                                    |
| `docs`     | 📃   | 修改文档                                                    |
| `style`    | 🌈   | 格式修改（不影响功能，例如空格、分号等格式修正）            |
| `refactor` | 🦄   | 代码重构                                                    |
| `perf`     | 🎈   | 代码优化 / 性能优化                                         |
| `test`     | 🧪   | 新增测试 / 修改测试                                         |
| `build`    | 🔧   | 构建依赖更改                                                |
| `ci`       | 🐎   | 更改 CI 配置文件或者脚本                                    |
| `chore`    | 🐳   | 更改构建流程、或者增加依赖库、工具等                        |
| `revert`   | ↩    | 代码回退                                                    |

## 示例

```
🎉 init: initialize gfa-core project
✨ feat(auth): add JWT login
🐞 fix(user): correct pagination total count
📃 docs(readme): update local development guide
🌈 style(web): format tailwind class order
🦄 refactor(core): extract common middleware
🎈 perf(dao): add index for deleted_at filter
🧪 test(auth): add login unit tests
🔧 build(core): bump gf to v2.10.2
🐎 ci: add github actions workflow
🐳 chore: add commitlint config
↩ revert: revert "feat(auth): add JWT login"
```

## 注意事项

- **提交前必须经用户交互式确认**（见上文「提交前必须经用户交互式确认」），未经同意不得自行提交。
- 优先选择能准确表达改动意图的最小类型。
- 当一次改动同时包含多个类型时，建议拆分为多个提交；若必须合并，使用最主要的类型。
- 图标与类型之间保留一个空格，例如 `✨ feat(auth): ...`。
