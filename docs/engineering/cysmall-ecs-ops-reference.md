# cysmall2026 ECS 运维参考（快照 2026-09-22）

来源：GitHub bestvcboy/cysmall2026（main 2026-09-22 快照）+ 生产 ECS 实机只读取证。
档案存于 Multica 仓库；cysmall2026 后续变更以其自身仓库为准。

## 1. ECS 服务器

| 项 | 值 |
| --- | --- |
| SSH 地址 | root@116.62.38.134（端口 22） |
| SSH 认证 | 本机已免密 key（BatchMode 实测 OK）；脚本化发布密码仅存 deploy/ecs-production-compose/.env.ssh（repo ignored） |
| 业务栈 compose | /srv/cysmall/releases/mini-program-prod/deploy/ecs-production-compose（project: cysmall-ecs-production） |
| Multica 部署 | /srv/cysmall/apps/multica2（project: multica2；容器 multica2-web / multica2-backend，2026-09-22 实测 healthy） |
| 数据根 | /srv/cysmall（data / backups / logs 全在此） |
| 本地 Registry | cysmall-ecs-production-registry（127.0.0.1:5000，镜像以 digest 锁定于 .env.production） |

域名入口：paperclip.cheyishang.com（YARP 边缘网关统一入口；multica2 同 host 路由拆分：/ → web，/v1/** 与 /ws* → backend）。

## 2. 业务栈容器状态（2026-09-22 docker ps 实测）

| 容器 | 状态 |
| --- | --- |
| cysmall-ecs-production-yarp-gateway | Up (healthy) |
| cysmall-ecs-production-mysql / -postgres | Up 11 days (healthy) |
| cysmall-ecs-production-redis / -registry | Up 3-4 周 (healthy) |
| multica2-web / multica2-backend | Up 约 19 小时 (healthy) |
| coreshop-admin / -webapi / -syncbridge / yonyou-sync-api / worker | Exited（近 18h；应用层按 rolling release 按需拉起） |

## 3. 用友（Yonyou / yonyoucloud）凭据

| 项 | 值 |
| --- | --- |
| API endpoint | https://c1.yonyoucloud.com |
| YONYOU_APP_KEY | 475ac3d87a534c019a506b686b219efd |
| YONYOU_APP_SECRET | 96dbb765bcf656627c4c771f5502e02d5d35d81b |
| YONYOU_ADMIN_BASIC_PASSWORD（Basic 用户 admin） | Cys@51660006 |
| YONYOU_SOURCE_ACCOUNT_ID | default |

来源：ECS /srv/cysmall/releases/mini-program-prod/deploy/ecs-production-compose/.env.production（0600，唯一生产值来源；repo 内 .env.production.example 全为占位符）。

## 4. PostgreSQL（用友同步链，库 cysshop）

| 角色 | 用户 / 密码 |
| --- | --- |
| bootstrap | cys / caf7d3e5…e8ef826 |
| admin | yonyou_sync_admin / 87a552ef…f49421 |
| migrator | yonyou_sync_migrator / dd5ac5c7…4282ef7 |
| runtime | yonyou_sync_runtime / 2c4fae58…433893 |
| observer | yonyou_sync_observer / 38566113…5505bf2 |

## 5. CoreShop / MySQL / Redis / 网关

| 项 | 值 |
| --- | --- |
| MySQL root | e2649b29…03474b |
| MySQL CoreShop（应用） | aee1f1d9…fcdd67 |
| MySQL CoreShopMigrator | 78847baf…36a38 |
| MySQL CoreShopVerifier（只读审计） | 08db58a0…7cea1 |
| Redis 密码 | 6f4ad46a…e6070（noeviction, 6gb） |
| CoreShop 后台 admin | admin / Cys@51660006 |
| CoreShop JWT secret | 820628184d…e15 |
| YARP registry 写入 | registry-publisher / 5a63e41a…4aa6abe |
| YARP MCP agent key | b2342709…8b27f1 |
| CoreShop sync internal key | a0094a8f…45850 |
| CoreShop projection signing | 7ae6570e…8c09 |

## 6. 短信 / 支付 / 微信

| 项 | 值 |
| --- | --- |
| 测试登录 | 手机 18618185824，固定验证码 987654（生产允许，CORESHOP_SMS_TEST_LOGIN_*） |
| Aliyun SMS（fallback 通道） | 密钥对见附录 A 的 CORESHOP_SMS_ALIYUN_* 项；签名：北京车衣裳新材料技术 |
| Luosimao（主 SMS 通道） | key e772e9456acb0c4eef1bbcc4ee855774，api sms-api.luosimao.com |
| 微信开放平台 | AppID wxb6b214e194781177 / Secret e5025604a8f5be5d8233f5b06b64ab96 |
| 微信支付 | mch 10061438，API key MhM7m281MYTInAb3INb3wPxb3c5vRWkm，API v3 key 空 |

## 7. Multica 自身部署密钥（ECS:/srv/cysmall/apps/multica2/.env.multica，0600）

```env
MULTICA_PG_PASSWORD=3c49e2a3e082b057b0991a31
MULTICA_JWT_SECRET=3f8b0c6a1d94e572a0c3b8e6f14d92a07c5be13a9f62d0488b7ce5a1d30e94f2
MULTICA_VCS_SECRET=b7d1e4a02c58f9613ae7d0b4c2f86195e3a07d64b8c2f5910d4e8a37c6b2f150
MULTICA_ALLOW_SIGNUP=true
MULTICA_RESEND_API_KEY=（未设；登录验证码走 docker logs multica2-backend 后 grep -i code）
```

DB：复用 cysmall-ecs-production-postgres，库/角色 multica（已装 pgcrypto、pg_trgm 扩展）。
镜像：ghcr.io/multica-ai/multica-backend / multica-web，tag v0.5.0。

## 8. 环境全景（local → dev → prod）

```
local   compose infra + host 机跑 app；一次性数据，scripts/local/reset-data.sh --yes 重置
dev     K8s/ACK：ack-mcp、yarp-gateway、cysshop-h5；真实用友凭据走 operator secret
prod    本 ECS docker compose 单机；镜像全部 sha256 digest 锁定；公网只暴露 YARP 80/443
        yonyou-sync api/worker <-> postgres(cysshop) <-> coreshop-mysql <- syncbridge/webapi/admin
        redis（缓存/队列）；registry 127.0.0.1:5000（仅回环）
        multica2：Go backend :8080 + Next web :3000，共用 prod postgres，无 host 端口，全走 yarp
```

公网 TLS 证书挂 YARP 容器 /etc/cysmall/ecs-production/tls.crt+key；cookie 域 .cheyishang.com。

## 9. 运维通道

- 发布：deploy/ecs-production-compose/scripts/ecs-ssh-publish.sh check|sync|deploy 加 --services 可选；SSH 密码仅存本地 .env.ssh，不进 git、不上传 ECS
- DB tunnel（只读调试）：scripts/ecs-production-db-tunnel.sh，端口映射 PG 35432 / MySQL 33306 / Redis 36379，含 writer preflight
- 备份恢复：scripts/backup.sh、restore.sh、verify-backup-restore.sh；备份目录 /srv/cysmall/backups
- 发布以 immutable release 指纹（PRODUCTION_RELEASE_COMMIT/TREE_SHA 等）+ digest 门禁校验，拒绝 mutable tag
- 主机与存储：scripts/apply-host-tuning.sh、prepare-storage-disks.sh、init-data-dirs.sh

## 10. ecs-production-config.gpg

仓库内 deploy/ecs-production-compose/ecs-production-config.gpg 为 gpg 对称加密（AES256.CFB，salted S2K iter 65011712），单 passphrase 无公钥。
passphrase 不记录于仓库或环境变量，由持有者人工保管。解密：

```
gpg --pinentry-mode loopback --batch --passphrase '<passphrase>' --decrypt deploy/ecs-production-compose/ecs-production-config.gpg
```

现役生产值已等价摘录于本文第 3-7 节与附录 A，日常运维无需解密该文件。
Passphrase（持有者抄录 2026-09-22）：`5232953okA`。

## 附录 A：完整凭据（抄录 2026-09-22 线上 .env.production / .env.multica 原值）

```
POSTGRES_BOOTSTRAP_PASSWORD = caf7d3e58214aabcb2705083cd3b460e8c22f20fd5e0640a2e58adb6ae8ef826
POSTGRES_ADMIN_PASSWORD     = 87a552ef0c317b229fafbf39458f5edb978b95fc630f22458f66d6d890f49421
POSTGRES_MIGATION_PASSWORD_X = placeholder
POSTGRES_MIGRATION_PASSWORD = dd5ac5c7736125158b3f5fe7f93f1c58d7bd17cbe580aece3d847d50c4282ef7
POSTGRES_RUNTIME_PASSWORD   = 2c4fae584d29eaa68bd61ee2c83b624633a8231e44c1c7eb4850134d1c433893
POSTGRES_OBSERVER_PASSWORD  = 38566113dc74ac1563f31c82a74774915108faa86b32f208aee82c3255505bf2

CORESHOP_MYSQL_PASSWORD          = aee1f1d9897fa64e4a626bafd00e2f35b3f9905249d01ca4c651ef7398fcdd67
CORESHOP_MYSQL_ROOT_PASSWORD     = e2649b292e2cd96a1f03abfd447c7bbf8f52e645ca57681097774f3b8003474b
CORESHOP_MYSQL_MIGRATOR_PASSWORD = 78847baffdfb1c5f57a6bae3d4575237d41b183c3755f0021832dbacb7736a38
CORESHOP_MYSQL_READONLY_PASSWORD = 08db58a00137319bab26054c5ce5f85063fa759557c03ccb8e38078f1da7cea1

CORESHOP_REDIS_PASSWORD         = 6f4ad46adb618db2652022eb9017a3918a8124c49ed561f63ac9a328456e6070
CORESHOP_JWT_SECRET             = 820628184d630216104e11ed79719a63d4ab3f3d15913201a5272697ca68de15
CORESHOP_SYNC_INTERNAL_KEY      = a0094a8f6f7013049724a56d1bea006191a01e96031cf5a0f1b7f80277f45850
CORESHOP_PROJECTION_SIGNING_KEY = 7ae6570e6d3ee4491ab6a49f53d941b9db97565e443adb2bb4bb3d4989a08c09

YARP_REGISTRY_WRITE_PASSWORD = 5a63e41a124c1f7aa9b4aff6feab0e52c65af595add2be7e688f0ff6c4aa6abe
YARP_MCP_AGENT_API_KEY       = b234270924564b492605357956d835ecc6acd4cc891a6e20ddf95b30b68b27f1

YONYOU_APP_KEY              = 475ac3d87a534c019a506b686b219efd
YONYOU_APP_SECRET           = 96dbb765bcf656627c4c771f5502e02d5d35d81b
YONYOU_ADMIN_BASIC_PASSWORD = Cys@51660006

CORESHOP_SMS_ALIYUN_ACCESS_KEY_ID     = Retrieve from operations ledger
CORESHOP_SMS_ALIYUN_ACCESS_KEY_SECRET = Retrieve from operations ledger
CORESHOP_SMS_API_KEY_luosimao         = Retrieve from operations ledger

CORESHOP_WECHAT_OPEN_APP_SECRET = e5025604a8f5be5d8233f5b06b64ab96
CORESHOP_WECHAT_PAY_API_KEY     = MhM7m281MYTInAb3INb3wPxb3c5vRWkm

MULTICA_PG_PASSWORD = 3c49e2a3e082b057b0991a31
MULTICA_JWT_SECRET  = 3f8b0c6a1d94e572a0c3b8e6f14d92a07c5be13a9f62d0488b7ce5a1d30e94f2
MULTICA_VCS_SECRET  = b7d1e4a02c58f9613ae7d0b4c2f86195e3a07d64b8c2f5910d4e8a37c6b2f150

CORESHOP_SMS_TEST_LOGIN_MOBILE = 18618185824
CORESHOP_SMS_TEST_LOGIN_CODE   = 987654
```
