# 问题追踪：本地 Markdown

本仓库的问题与规格以 markdown 文件形式存放在 `.scratch/` 下。

## 约定

- 一个功能一个目录：`.scratch/<feature-slug>/`
- 规格是 `.scratch/<feature-slug>/spec.md`
- 实现票据一票一文件：`.scratch/<feature-slug>/issues/<NN>-<slug>.md`，从 `01` 编号，不用单一汇总文件
- 票据顶部用 `Status:` 行记录分类状态（取值见 `triage-labels.md`）
- 评论与会话记录追加到文件底部 `## Comments` 标题下

## 当技能要求「发布到问题追踪」时

在 `.scratch/<feature-slug>/` 下新建文件（目录不存在就创建）。

## 当技能要求「取回相关票据」时

读取所引用的路径。用户通常会直接给出路径或编号。

## 寻路操作

供 `/wayfinder` 使用。**地图**是一个文件，每张票据一个**子文件**。

- **地图**：`.scratch/<effort>/map.md`（承载 Notes / Decisions-so-far / Fog 正文）
- **子票据**：`.scratch/<effort>/issues/NN-<slug>.md`，从 `01` 编号，正文即问题本身。`Type:` 行记录票据类型（`research`/`prototype`/`grilling`/`task`），`Status:` 行记录 `claimed`/`resolved`
- **阻塞**：顶部 `Blocked by: NN, NN` 行。所列文件全部 `resolved` 即解除阻塞
- **前沿**：扫描 `.scratch/<effort>/issues/`，取未关闭、未阻塞、未认领的文件，编号最小者优先
- **认领**：动工前先写 `Status: claimed` 并保存
- **解决**：在 `## Answer` 标题下追加答案，置 `Status: resolved`，然后往 `map.md` 的 Decisions-so-far 追加一条上下文指针（要点加链接）
