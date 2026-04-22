# web/console（只读镜像）

本目录是用户控制台前端资源的**本地查阅镜像**，用于在 IDE 中快速浏览和 diff。

## 唯一源位置

构建产物的**唯一来源**是：

```
internal/interfaces/api/static/
```

该目录通过 Go 的 `//go:embed` 在 `internal/interfaces/api/router.go` 被打包进二进制。
`web/console/` **不会**被任何构建步骤读取。

## 修改规则

1. 所有前端修改必须先改动 `internal/interfaces/api/static/`。
2. 改完后运行 `make sync-web-console` 将改动同步到本目录，保持两份一致。
3. CI / 本地可以运行 `make check-web-console-sync` 校验是否同步。

## 为什么保留这份镜像

- 方便在 IDE 文件树中直接展开前端资源（相比 `internal/` 深埋的路径）
- 便于对外分享 / 对比 / 静态预览，无需启动 Go 服务

如果未来团队不再需要本目录，可以整体删除并在 Makefile 中移除 `sync-web-console` / `check-web-console-sync` 目标。
