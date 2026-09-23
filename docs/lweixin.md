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

## Fork 接线清单

扩展实现归属 `server/internal/integrations/lweixin/`、`server/internal/handler/lweixin.go`、`packages/core/lweixin/`、`packages/views/settings/components/lweixin-tab.tsx` 和 `packages/views/lweixin/`。共享代码只接入这些模块，不修改其他渠道的处理逻辑。

| 接入点 | 上游同步后重验 |
| --- | --- |
| `server/cmd/server/router.go`、`server/internal/handler/handler.go` | 密钥配置、channel resolver/factory、出站订阅、成员可读与管理员可写的安装路由、登录后绑定路由仍接线。 |
| `server/pkg/protocol/events.go`、`packages/core/realtime/use-realtime-sync.ts` | 创建和撤销事件仍使工作区安装列表失效。 |
| `server/migrations/535_issue_origin_lweixin_chat.up.sql` | 上游迁移编号和 `issue.origin_type` 约束不冲突。 |
| `packages/core/api/client.ts`、`packages/core/api/schemas.ts`、`packages/core/types/lweixin.ts` | 列表、连接、断开、绑定仍按兼容 schema 解析；安装状态及可用标记保留。 |
| `packages/views/agents/components/agent-overview-pane.tsx`、`packages/views/agents/components/tabs/integrations-tab.tsx` | 仅 LWEIXIN 启用时 Web/桌面共用的智能体导航显示集成入口，管理员能连接，已有连接仍可查看或断开。 |
| `packages/views/settings/components/integrations-tab.tsx`、`apps/web/app/lweixin/bind/page.tsx` | 设置入口和连接状态仍可达；浏览器绑定链接仍落到绑定页。 |

回归检查：`pnpm --filter @multica/views exec vitest run agents/components/agent-overview-pane.test.tsx settings/components/integrations-tab.test.tsx settings/components/lweixin-tab.test.tsx`，以及同步后的 `go build ./...`（在 `server/`）。
