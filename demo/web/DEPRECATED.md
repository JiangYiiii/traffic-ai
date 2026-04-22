# demo/web（已废弃）

本目录保留的是**旧版**用户控制台前端原型与对外爬取分析文档，已于用户端 UI 原型替换后归档。

## 现状

- 不被 Go 构建引用（`//go:embed` 指向 `internal/interfaces/api/static/`）
- 不被任何脚本、CI 步骤引用
- 仅作为历史参考与对外竞品分析（见 `4tk-console-scrape.md`）存在

## 当前的前端源

- 唯一构建源：`internal/interfaces/api/static/`
- 本地镜像副本：`web/console/`（详见 `web/console/README.md`）
- 最新原型参考：`demo/userClient/`

## 清理建议

如果团队后续不再需要保留历史与竞品爬取资料，可以直接删除整个 `demo/web/` 目录，不会影响构建与运行时。
