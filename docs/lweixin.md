# 微信（LWEIXIN）连接

在自己的 Multica 服务中打开智能体 → 连接与扩展 → 微信（LWEIXIN）→ 连接微信。
填写已登录微信的 LWEIXIN 服务地址和 API token。地址保留网关分配的完整路径，例如 `https://wchat.cheyishang.com/xxx/xxxx/xxxxx`；不要附加 `/api/status`。
Multica 后端会访问该地址下的 `/api/status`，获取微信号 wxid；成功后保存连接。

一个工作区可连接多个微信号。同一个微信号不能同时连接到不同工作区；需要先断开原连接。
新连接初始静默：收到的文字消息按微信号、朋友或群保存，管理员可在设置中查看历史，分别配置私聊与群聊默认规则以及具体会话的覆盖规则。当前版本尚未启用隔离执行，即使选择了智能体也不会自动回复；静默期间不发送绑定提示，也不会在启用接待后补回复旧消息。
升级前已有的连接暂时保留原有单智能体和发信人绑定流程。不要把新连接的规则视为旧连接已完成迁移。

连接列表和断开操作位于设置 → 集成 → 微信（LWEIXIN）；只有工作区 owner/admin 可以管理连接。

## 部署与排查

LWEIXIN 消息轮询由 ECS 上的 Multica 后端运行；接待执行尚未启用。现有工作站进程与其主人共用系统身份，可读取本机凭据；独立的接待 API token 不构成操作系统隔离。在这项边界得到验证前，不应开启面向未绑定微信发信人的智能体执行。
后端需要设置 `MULTICA_LWEIXIN_SECRET_KEY`，并能访问 LWEIXIN 服务。
连接失败时检查服务地址、API token、微信登录状态与 `/api/status` 返回的 wxid。
旧连接的绑定链接过期时，回到微信重新发送消息获取新链接。

本扩展属于 bestvcboy/multica fork。后端隔离在 `server/internal/integrations/lweixin/`，共享前端通过独立模块和少量入口接线集成；同步上游后仍需验证这些入口。

## Fork 接线清单

扩展实现归属 `server/internal/integrations/lweixin/`、`server/internal/handler/lweixin.go`、`packages/core/lweixin/`、`packages/views/settings/components/lweixin-tab.tsx` 和 `packages/views/lweixin/`。共享代码只接入这些模块，不修改其他渠道的处理逻辑。新连接的记录与管理不依赖通用渠道的成员身份路由。

| 接入点 | 上游同步后重验 |
| --- | --- |
| `server/cmd/server/router.go`、`server/internal/handler/handler.go` | 密钥配置、channel resolver/factory、出站订阅、成员可读与管理员可写的安装路由、登录后绑定路由仍接线。 |
| `server/internal/integrations/lweixin/history.go`、`routing.go`、`server/internal/handler/lweixin.go` | 新连接按入口与会话去重记录文字；路由管理与历史读取仍要求 owner/admin，旧连接继续使用原处理器。 |
| `server/migrations/536`–`542`、`server/cmd/migrate/main.go` | 会话、文字和路由表及独立并发索引的中断恢复映射仍匹配；新增迁移编号不与上游冲突。 |
| `server/pkg/protocol/events.go`、`packages/core/realtime/use-realtime-sync.ts` | 创建和撤销事件仍使工作区安装列表失效。 |
| `server/migrations/535_issue_origin_lweixin_chat.up.sql` | 上游迁移编号和 `issue.origin_type` 约束不冲突。 |
| `packages/core/api/client.ts`、`packages/core/api/schemas.ts`、`packages/core/types/lweixin.ts` | 列表、连接、断开、绑定仍按兼容 schema 解析；安装状态及可用标记保留。 |
| `packages/views/agents/components/agent-overview-pane.tsx`、`packages/views/agents/components/tabs/integrations-tab.tsx` | 仅 LWEIXIN 启用时 Web/桌面共用的智能体导航显示集成入口，管理员能连接，已有连接仍可查看或断开。 |
| `packages/views/settings/components/integrations-tab.tsx`、`apps/web/app/lweixin/bind/page.tsx` | 设置入口和连接状态仍可达；浏览器绑定链接仍落到绑定页。 |
| `packages/views/settings/components/lweixin-tab.tsx`、`packages/core/lweixin/` | Web/桌面共用管理员历史与路由设置；列表分页、会话范围和静默提示仍有效。 |

回归检查：`pnpm --filter @multica/views exec vitest run agents/components/agent-overview-pane.test.tsx settings/components/integrations-tab.test.tsx settings/components/lweixin-tab.test.tsx`，以及同步后的 `go build ./...`（在 `server/`）。
