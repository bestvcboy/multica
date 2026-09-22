# 微信（LWEIXIN）连接

在自己的 Multica 服务中打开智能体 → 连接与扩展 → 微信（LWEIXIN）→ 连接微信。
填写已登录微信的 LWEIXIN 服务地址和 API token。地址保留网关分配的完整路径，例如 `https://wchat.cheyishang.com/xxx/xxxx/xxxxx`；不要附加 `/api/status`。
Multica 后端会访问该地址下的 `/api/status`，获取微信号 wxid；成功后保存连接。

一个工作区可连接多个微信号，分别在对应智能体页面配置。同一个微信号不能同时连接到不同智能体或工作区；需要先断开原连接。
微信号 A 是智能体的收发消息账号。用户用微信号 B 向 A 发消息，Multica 将消息交给 A 关联的智能体，并通过 A 回复。
首次对话按提示打开绑定链接，登录属于该工作区的 Multica 账号，绑定发信人的身份。此步骤不更改 A 与智能体的关联。

连接列表和断开操作位于设置 → 集成 → 微信（LWEIXIN）；只有工作区 owner/admin 可以管理连接。

## 部署与排查

LWEIXIN 消息轮询与回复由 ECS 上的 Multica 后端运行；智能体运行时仍在所选工作站执行任务。
后端需要设置 `MULTICA_LWEIXIN_SECRET_KEY`，并能访问 LWEIXIN 服务。
连接失败时检查服务地址、API token、微信登录状态与 `/api/status` 返回的 wxid。
绑定链接过期时，回到微信重新发送消息获取新链接。

本扩展属于 bestvcboy/multica fork。后端隔离在 `server/internal/integrations/lweixin/`，共享前端通过独立模块和少量入口接线集成；同步上游后仍需验证这些入口。
