# LWEIXIN 前端发布记录

2026-09-22 已部署到 `https://paperclip.cheyishang.com`，ECS Compose 项目为 `multica2`。

- 源码：bestvcboy/multica `vc/main`，提交 `04a569f87`。
- 前端镜像：`multica2-web:vc-lweixin-20260922`。
- 后端镜像：`multica2-backend:vc-lweixin-20260922`。
- 两个容器部署后均为 healthy；后端日志确认 LWEIXIN 集成与逐连接轮询已启用。
- 公网 `/lweixin/bind` 返回 HTTP 200；未登录调用连接列表返回 HTTP 401。
- 验证：全仓前端类型检查通过；251 项界面/翻译测试及 97 项 API 测试通过；改动文件 lint 通过；前后端 Docker 构建成功。
- 未执行真实微信收发测试、完整 E2E 或全量 Go 测试。浏览器自动化读取页面超时，因此没有将 HTTP 检查当作视觉验证通过。
- 本次发布更新服务端及 Web，不包含 Windows 安装包重建或本机客户端更新。

## 回滚

ECS 工作目录：`/srv/cysmall/apps/multica2`。发布前配置备份为 `docker-compose.yml.pre-lweixin-20260922`，旧前后端镜像 tag 为 `vc-cf8fecef`。
需要回滚时，仅将当前 Compose 中这两个服务的 image 改回旧 tag，再运行 `docker compose --env-file .env.multica up -d --pull never backend frontend`。
保留当前 `.env.multica`；本次未新增数据库迁移。

使用入口见 [微信连接指南](../lweixin.md)。
