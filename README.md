# NodeLoc Store

> 参考 Dujiao-Next 运营与交付模式，基于 NodeLoc OAuth2 + Nodeloc Payments 的数字商品商店 · v1.0.0

一个完整可商用的数字商品与自动发卡平台：保留 **NodeLoc OAuth2 + 邮箱注册登录**，支付统一接入 **Nodeloc Payments**，支持卡密自动交付、人工交付、订单履约追踪、角色权限、运营设置与审计。当前衍生版本统一定义为 **v1.0.0**。

![license](https://img.shields.io/github/license/kaoqy/Nodeloc-Store)

## ✨ 功能特性

- 🚀 **首次访问即安装** — 引导式配置数据库 + Admin 账号 + NodeLoc OAuth + 支付凭据
- 🔐 **双通道登录** — NodeLoc OAuth2 一键登录 / 邮箱注册登录，Scope 感知（`email` 未授权时自动隐藏）
- 💎 **精美 UI** — Tailwind + 玻璃拟态 + 渐变设计，深色主题，响应式
- 📦 **商品管理** — 卡密商品与人工交付商品、图片、定价、交付说明、联系方式要求、库存可见性和上下架
- 🎫 **卡密系统** — 批量导入（每行一个）、状态管理（可用/已售/禁用）、库存自动同步
- 💰 **Nodeloc Payments** — 所有订单统一走 Nodeloc Payments，支持多种回调参数、HMAC-SHA256 验签和幂等履约
- 🚚 **统一履约** — 卡密自动发货、缺货等待补货、人工交付、交付内容与备注、用户侧履约状态查询
- 👥 **角色权限** — 超级管理员、管理员、运营、客服和普通用户五级权限
- 🎁 **用户运营** — 每日签到、积分、连续签到奖励、站点公告与客服信息
- 📊 **Admin 后台** — 概览统计、商品/卡密/订单/用户管理、操作审计日志、退款
- 🛠️ **OpenResty 反代** — 适合用 OpenResty 跑其他服务、复用现有 vhost 的部署场景
- 🔒 **安全** — bcrypt 密码哈希、回调 HMAC 验签、Casbin RBAC、操作审计日志

## 📸 截图

<details>
<summary>商店首页</summary>

玻璃拟态深色主题，渐变标题，卡片式商品列表，支持搜索与分页。

</details>

<details>
<summary>Admin 后台</summary>

侧边栏导航、数据卡片、最新订单与日志一目了然。

</details>

## 🚀 快速部署

### 前置要求

- 一台 Linux 服务器（Ubuntu 22.04 / Debian 12）
- Docker（推荐）；或使用外部 MySQL 5.7+ / MariaDB 10.3+（可选，默认内置 SQLite）
- 已配好 OpenResty（含 SSL，Let's Encrypt 推荐）
- NodeLoc 论坛账号：**白银会员 TL1**及以上才能创建支付应用；**OAuth 需 TL2 黄金会员**

### Step 1 · 准备数据库（可选）

默认使用内置 **SQLite**（初始化时存放在 `./data` 卷里），零配置即可上线，单店中小流量完全够用。

只有当你需要外部 MySQL/MariaDB 时才执行：

```bash
# Ubuntu / Debian
sudo apt update
sudo apt install -y mariadb-server
sudo mysql_secure_installation

# 创建数据库和用户
sudo mysql -e "
  CREATE DATABASE nodeloc_store CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
  CREATE USER 'store_user'@'localhost' IDENTIFIED BY '强密码填这里';
  GRANT ALL ON nodeloc_store.* TO 'store_user'@'localhost';
  FLUSH PRIVILEGES;
"
```

如果是远程 DB，把 `'store_user'@'localhost'` 改为 `'store_user'@'%'`，并确保数据库服务器 `bind-address` 与防火墙允许应用服务器连接。数据库连接信息在初始化向导里填写，无需任何配置文件。

### Step 2 · 在 NodeLoc 创建应用

> NodeLoc 创建应用时填的回调地址，**必须**是你 OpenResty 反代出来的 HTTPS 域名（不能是 `http://127.0.0.1:8080`）。

#### 2.1 OAuth 应用

访问 <https://www.nodeloc.com/oauth-provider/applications> → 创建应用：

| 字段 | 填什么 |
|---|---|
| 应用名称 | 你的商店名 |
| 网站地址 | `https://你的域名` |
| 回调地址 | `https://你的域名/api/v1/auth/oauth/callback` |
| 权限范围 | 勾选 `openid`（必选）、`profile`、`email`（需审核） |

保存后记录 **Client ID** 和 **Client Secret**（只显示一次）。

#### 2.2 支付应用

访问 <https://www.nodeloc.com/payment/applications> → 创建应用：

| 字段 | 填什么 |
|---|---|
| 应用名称 | 你的商店名 |
| 网站地址 | `https://你的域名` |
| 回调地址 | `https://你的域名/api/v1/payment/callback` |

保存后记录 **Payment ID** 和 **Secret Key**（只显示一次）。

#### 2.3 回调接口一览（与代码路由一致）

| 用途 | 方法 | 接口路径 | 谁来调用 | 之后发生什么 |
|---|---|---|---|---|
| OAuth2 登录回调 | `GET` | `/api/v1/auth/oauth/callback` | NodeLoc OAuth 授权后浏览器跳转 | 校验 `state` + 换 token → 302 回商店前端 `/oauth/callback`（token 放 URL fragment，不落日志）；AJAX 请求则直接返回 JSON |
| 支付结果回调 | `GET` | `/api/v1/payment/callback` | NodeLoc 支付完成后浏览器跳转 | HMAC-SHA256 验签 → 幂等履约发卡 → 302 到 `/orders/{订单号}` |
| 支付结果回调 | `POST` | `/api/v1/payment/callback` | 服务端推送式通知 | 同上验签与履约，返回 `{"success":true}` |
| 买家查单 | `POST` | `/api/v1/payment/orders/{订单号}/reconcile` | 商店前台「我已支付，去确认」 | 用商户密钥主动向 NodeLoc 查询该单 → 已付则当场入账发卡，返回最新订单 |
| 商家查单 | `POST` | `/api/v1/admin/orders/{订单号}/reconcile` | 后台订单详情的「查单对账」 | 同上，管理员可对任意待支付订单执行 |

> 回调验签失败时商店不会直接判定「没付款」：签名候选（原密钥与 SHA-256 十六进制密钥、含空参与不含空参）全部不匹配后，会自动用商户密钥调用 NodeLoc 的查询接口核实这一单，NodeLoc 记为已付就当场入账并发卡。也就是说，**买家已付款但回调签名对不上时不需要再付一次**；后台设置里填错 Secret Key 时，这条路径同样会把订单核对出来。

> 两个地址都要**逐字**填进 NodeLoc 控制台（含 `/api/v1` 前缀），并与你商店初始化时填的域名完全一致；初始化向导和后台设置页的回调地址留空即自动生成，输入框占位符显示的就是完整地址，照抄到 NodeLoc 即可。
> 前端还有 `/oauth/callback`、`/orders` 等路由属于商店自己的页面，**不要**填到 NodeLoc 的回调地址里。

### Step 3 · 启动商店（Docker）

推荐直接使用 Docker Hub 已发布镜像一键部署（应用监听 **8080**，SQLite 数据落在当前目录 `./data`，商品图片落在 `./uploads`）：

```bash
docker run -d --name nodeloc-store --restart unless-stopped \
  -p 8080:8080 \
  -v "$PWD/data:/app/data" \
  -v "$PWD/uploads:/app/uploads" \
  kaoqy666/nodeloc-store:v1.0.0
```

跟随最新版：把上面的镜像名换成 `kaoqy666/nodeloc-store:latest` 即可。

> 如果要在初始化向导里连接 1Panel 管理的 MariaDB（容器名如 `1Panel-mariadb-oxyE`），需要让商店加入数据库所在的 Docker 网络：加 `--network 1panel-network`（网络名用 `docker inspect 1Panel-mariadb-oxyE --format '{{range $name, $_ := .NetworkSettings.Networks}}{{$name}}{{end}}'` 查询）。用默认 SQLite 则完全不需要任何网络配置。

如果需要从源码构建，也可以使用 Docker Compose：

```bash
git clone https://github.com/kaoqy/Nodeloc-Store.git
cd Nodeloc-Store
docker compose up -d --build
```

> Compose 默认只起一个 `store` 容器并使用内置 SQLite，无外部依赖。需要连 1Panel 数据库网络时，编辑 `docker-compose.yml` 末尾已注释好的 `networks` 段即可。

启动后访问 `http://IP:8080/admin/` 进入初始化向导。查看日志：

```bash
docker logs -f nodeloc-store          # docker run 部署
docker compose logs -f store          # compose 部署
```

### Step 4 · OpenResty 反代 + SSL

在 OpenResty 配置目录加一个 server 块（路径以你的环境为准，比如 `/usr/local/openresty/nginx/conf/conf.d/store.conf`）：

```nginx
server {
    listen 443 ssl http2;
    server_name your-domain.com;

    ssl_certificate     /path/to/fullchain.pem;
    ssl_certificate_key /path/to/privkey.pem;

    client_max_body_size 8M;

    # 反代到 Docker 容器里 Go 服务监听的 8080（前端静态资源已内嵌在镜像中）
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_redirect off;
        proxy_http_version 1.1;
        proxy_read_timeout 60s;
    }
}

server {
    listen 80;
    server_name your-domain.com;
    return 301 https://$host$request_uri;
}
```

```bash
sudo openresty -t && sudo openresty -s reload
```

> 商店无需在 nginx 里挂静态资源：页面、CSS/JS、图片都由容器内的服务直接吐出。数据卷 `./data:/app/data` 保存 SQLite 库与初始化配置，`./uploads:/app/uploads` 存放商品图片等持久文件——把图片放进宿主机的 `./uploads/`，在后台商品表单的「图片路径」里填 `/uploads/文件名` 即可（应用本身不提供上传接口）。

### Step 5 · 应用内初始化向导

浏览器访问 `http://IP:8080`（配好域名后访问 `https://你的域名`），未初始化时会自动跳到管理后台的初始化向导 `/admin/setup`，三步完成：

1. **站点与数据库** — 商店名称、访问域名、数据库驱动（默认 SQLite；选 MySQL 时填 Step 1 的连接信息）
2. **NodeLoc 集成** — OAuth 的 Client ID / Secret（必填）+ 支付的 Payment ID / Secret Key（可稍后在设置中补填）
3. **管理员账号** — 创建首个管理员，保存后直接进入后台登录

> 提交时如果数据库连不通，会显示错误提示让你重填，**不会破坏配置**。
> **保存即生效**，不用重启容器；无需编辑任何 yml / ini 配置文件。
> 之后随时可在后台 **设置** 页修改，密钥以 `********` 掩码显示、保持掩码即不修改，还可一键测试 OAuth 与支付网关连通性。

### Step 6 · 验证支付

1. 用 Admin 账号登录后台 → **商品** → 新建一个商品 + 导入几个测试卡密
2. 退出登录，用另一个 NodeLoc 账号或邮箱注册普通用户
3. 下单购买 → 跳转到 NodeLoc 支付页 → 用积分支付
4. 支付完成后浏览器会跳转回你的 `/api/v1/payment/callback`，自动发货，卡密会显示在订单详情

## 🛠️ 常用运维

```bash
# 看容器日志
docker logs -f nodeloc-store          # docker run 部署
docker compose logs -f store          # compose 部署

# 重启容器
docker restart nodeloc-store

# 升级到新版本（版本号不变时直接重拉 tag 为 v1.0.0 的镜像即可）
docker pull kaoqy666/nodeloc-store:v1.0.0
docker restart nodeloc-store

# 源码构建部署时升级
cd Nodeloc-Store && git pull && docker compose up -d --build

# 备份（SQLite / 初始化配置 / JWT 密钥都在 ./data 目录）
cp -r data backup_$(date +%F)
# 备份外部 MariaDB 时改用：
mysqldump -u store_user -p nodeloc_store > backup_$(date +%F).sql

# 健康检查（OpenResty upstream 用）
curl -s http://127.0.0.1:8080/api/health
# {"status":"ok"}
```

## 🔒 安全建议

- ✅ OpenResty 必须配置 SSL，NodeLoc 强制要求回调为 HTTPS
- ✅ DB 用户只授予 `nodeloc_store` 库的权限，不要用 root
- ✅ 初始化时设置强管理员密码（至少 8 位）
- ✅ 定期备份 `./data` 目录（含 `bootstrap.json` 与 SQLite 库）与数据库
- ✅ OAuth `email` scope 需在 NodeLoc 审核通过；未通过时用户的邮箱将为空
- ✅ `data/bootstrap.json` 含 JWT 密钥，已在 `.gitignore` 中，不要提交到 Git

## 🧪 自测

```bash
go build ./... && go vet ./... && go test ./...   # 后端 + 架构约束测试
cd frontend/admin && npx vue-tsc --noEmit          # 前端类型检查
```

## 📁 项目结构

```
Nodeloc-Store/
├── cmd/server/            # Go 入口（swapHandler 热重建 + SPA 静态托管）
├── internal/
│   ├── config/            # 默认值 + data/bootstrap.json（驱动/DSN/端口/JWT）
│   ├── app/container/     # 依赖装配
│   ├── authz/             # Casbin RBAC
│   ├── models/            # 共享 GORM 模型与迁移
│   └── modules/           # identity / payment / catalog / notification / audit
│       └── system/        # 应用内初始化：status / install / settings / 连通测试
├── frontend/user/         # 商店 SPA（Vue3 + Tailwind，挂在 /）
├── frontend/admin/        # 后台 SPA + 初始化向导（挂在 /admin）
├── Dockerfile             # 多阶段：双 SPA + Go 二进制 → alpine
└── docker-compose.yml
```

## 🐛 故障排查

| 问题 | 解决 |
|---|---|
| 容器起不来 / 端口不通 | `docker logs nodeloc-store` 看报错；确认宿主端口映射是 `8080:8080`（旧文档的 5000 已废弃） |
| 向导卡在数据库步骤 | SQLite 时确认 `./data` 卷可写；外部 MariaDB 时确认容器能解析 DB 主机名（同网络或远程 IP）、端口开放、用户对库有权限 |
| 回调签名验证失败 | 后台 **设置** 里重新填 Payment ID / Secret Key（与 NodeLoc 控制台一致）；已付款的订单商店会自动向 NodeLoc 查单核实并入账，无需重复付款 |
| OAuth 登录失败 | NodeLoc 应用里的回调地址必须是 `https://你的域名/api/v1/auth/oauth/callback`，与设置页显示的完全一致 |
| 邮件没拿到 | NodeLoc OAuth `email` scope 需审核通过；未通过时 token 只有 `openid` |
| 卡密一直没发货 | Admin → 日志 中 `payment.stock_warning` 条目，确认有可用卡密 |
| 重启后丢失初始化状态 | `./data` 没挂载持久卷，按 Step 3 补上 `-v "$PWD/data:/app/data"` 重新起 |
| 上传图片 413 | OpenResty 的 `client_max_body_size` 与应用一致（默认 8M） |

## 📜 License

MIT

