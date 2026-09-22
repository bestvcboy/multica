# 自建版 Windows 安装包

默认服务器配置独立位于 `apps/desktop/src/shared/fork-runtime-config.ts`，连接 `https://paperclip.cheyishang.com`。
既有 `~/.multica/desktop.json` 是显式配置，优先于默认值；排查旧安装连接地址时先检查该文件。

使用 `apps/desktop/electron-builder.fork.yml` 继承上游打包配置，将更新仓库设为 `bestvcboy/multica`，避免自动更新回官方安装包。
在仓库根目录执行（需要 Node、pnpm、Git 和 Go 在 PATH 中）：

```powershell
pnpm --filter @multica/desktop package -- --win --x64 --publish never --config electron-builder.fork.yml -c.extraMetadata.version=0.5.2
```

产物为 `apps/desktop/dist/multica-cys-0.5.2-windows-x64.exe`。
没有 Windows 签名证书的本地构建不带发布者签名。
这条命令只生成安装包，不发布 GitHub Release，也不自动安装。
发布后续自建版本时使用同一 fork 配置，并按需向自己的仓库发布安装包与更新元数据。
