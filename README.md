# NodeLoc Store

> 基于 NodeLoc OAuth2 + 积分支付的卡密商店 · Flask + MySQL/MariaDB · Docker 部署

一个完整可商用的在线卡密商店：**NodeLoc OAuth 一键登录 + 邮箱注册双通道**，**NodeLoc 积分支付**收款，**Admin 后台管理商品/卡密/订单**，**玻璃拟态深色 UI**。

![license](https://img.shields.io/github/license/kaoqy/Nodeloc-Store)

## ✨ 功能特性

- 🚀 **首次访问即安装** — 引导式配置数据库 + Admin 账号 + NodeLoc OAuth + 支付凭据
- 🔐 **双通道登录** — NodeLoc OAuth2 一键登录 / 邮箱注册登录，Scope 感知（`email` 未授权时自动隐藏）
- 💎 **精美 UI** — Tailwind + 玻璃拟态 + 渐变设计，深色主题，响应式
- 📦 **商品管理** — 图片、定价、库存可见性、上下架
- 🎫 **卡密系统** — 批量导入（每行一个）、状态管理（可用/已售/禁用）、库存自动同步
- 💰 **NodeLoc 积分支付** — 浏览器跳转支付 + GET 回调验签（HMAC-SHA256）+ 自动发货
- 📊 **Admin 后台** — 概览统计、商品/卡密/订单/用户管理、操作审计日志、退款
- 🐳 **Docker 部署** — 商店只装应用容器，数据库用你自己的（MySQL/MariaDB），`instance/config.ini` 改动实时生效
- 🛠️ **支持任意数据库** — 你可以用本地装、远程托管、PlanetScale / 阿里云 RDS / 自建都行

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

- 一台 Linux 服务器（推荐 Ubuntu 22.04 / Debian 12）
- Docker + Docker Compose
- **一个可用的 MySQL 5.7+ 或 MariaDB 10.3+ 数据库**（本机、其他机器、RDS 都行）
- NodeLoc 论坛账号（**白银会员 TL1**及以上才能创建支付应用；OAuth 需 **TL2 黄金会员**及以上）

### Step 1 · 准备数据库

> 数据库**必须先准备好**（含已创建的空库和可远程连接的用户）。本项目不帮你装数据库。

#### 选项 A：服务器本地装 MariaDB

```bash
# Ubuntu / Debian
sudo apt update
sudo apt install -y mariadb-server
sudo mysql_secure_installation

# 创建数据库和用户
sudo mysql -e "
  CREATE DATABASE nodeloc_store CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
  CREATE USER 'store_user'@'%' IDENTIFIED BY '你的强密码';
  GRANT ALL ON nodeloc_store.* TO 'store_user'@'%';
  FLUSH PRIVILEGES;
"
```

> `CREATE USER ... @'%'` 允许从任意 IP 连接；如只想本机连接，改为 `@'localhost'`，部署时数据库主机填 `host.docker.internal` 或 `--network host`。

#### 选项 B：用现成的云数据库

阿里云 RDS / 腾讯云 MySQL / AWS RDS / PlanetScale 等都可以，只需要：
1. 创建一个空库（如 `nodeloc_store`），字符集 `utf8mb4`
2. 创建用户并授权该库
3. 把数据库的内网/公网地址记下来

#### 选项 C：用 Docker 自建一个（可选，不推荐生产）

```bash
docker run -d --name mysql \
  -e MYSQL_ROOT_PASSWORD=your_root_pass \
  -e MYSQL_DATABASE=nodeloc_store \
  -e MYSQL_USER=store_user \
  -e MYSQL_PASSWORD=store_pass \
  -p 3306:3306 \
  -v mysql_data:/var/lib/mysql \
  --restart unless-stopped \
  mariadb:10.11
```

### Step 2 · 在 NodeLoc 创建应用

> 提前在 [NodeLoc](https://www.nodeloc.com) 创建以下两个应用，把回调地址都填好。

#### 2.1 OAuth 应用

访问 <https://www.nodeloc.com/oauth-provider/applications> → 创建应用：

| 字段 | 填什么 |
|---|---|
| 应用名称 | 你的商店名（如 `我的商店`） |
| 网站地址 | `https://你的域名`（开发期填 `http://你的IP:5000`） |
| 回调地址 | `https://你的域名/auth/oauth/callback`（开发期填 `http://你的IP:5000/auth/oauth/callback`） |
| 权限范围 | 勾选 `openid`（必选）、`profile`、`email`（需审核） |

保存后记录 **Client ID** 和 **Client Secret**（只显示一次）。

#### 2.2 支付应用

访问 <https://www.nodeloc.com/payment/applications> → 创建应用：

| 字段 | 填什么 |
|---|---|
| 应用名称 | 你的商店名 |
| 网站地址 | `https://你的域名`（开发期填 `http://你的IP:5000`） |
| 回调地址 | `https://你的域名/payment/callback`（开发期填 `http://你的IP:5000/payment/callback`） |

保存后记录 **Payment ID** 和 **Secret Key**（只显示一次）。

### Step 3 · 部署商店

**一条命令**——只装应用容器，数据库用你自己的：

```bash
git clone https://github.com/kaoqy/Nodeloc-Store.git
cd Nodeloc-Store
docker compose up -d
```

> 💡 应用容器**默认会**尝试连接 `db:3306`（旧版默认值）；连不上会优雅降级到 SQLite，让你能在向导里测连通。真实数据库信息在下一步向导里填。

### Step 4 · 完成快速开始向导

首次访问 `http://你的IP:5000` 进入 5 步安装向导：

1. **数据库配置** — 填你 Step 1 准备好的 MySQL/MariaDB 连接信息
   - 主机：`localhost`、`192.168.x.x`、`your-rds-host.com` 都行
   - 端口：`3306`
   - 库名 / 用户 / 密码：同 Step 1
2. **商店信息** — 名称与标语
3. **NodeLoc OAuth** — 填入 Step 2.1 的 Client ID / Secret / Redirect URI / Scope
4. **NodeLoc 支付** — 填入 Step 2.2 的 Payment ID / Secret Key
5. **管理员账号** — 创建首个管理员

> 提交时如果数据库连不通，会显示错误提示让你重填，**不会破坏配置**。
> **保存即生效**，不用重启容器。

### Step 5 · 验证支付

1. 用 Admin 账号登录后台 → **商品** → 新建一个商品 + 导入几个测试卡密
2. 退出登录，用另一个 NodeLoc 账号或邮箱注册普通用户
3. 下单购买 → 跳转到 NodeLoc 支付页 → 用积分支付
4. 支付完成后浏览器会跳转回你的 `/payment/callback`，自动发货，卡密会显示在订单详情

> 💡 **开发/测试** 不想用 HTTPS？向导里有「允许 HTTP 回调」勾选框，勾上即可。
> 💡 **开发/测试** 可在 Admin → 设置页直接修改 OAuth / 支付配置，保存后**实时生效**，无需重启。

### Step 6 · HTTPS（生产环境建议开启）

NodeLoc 生产环境要求回调 URL 是 HTTPS。使用 Caddy 或 Nginx 反代：

#### Caddy（最简）

```caddy
你的域名 {
    reverse_proxy localhost:5000
}
```

#### Nginx

```nginx
server {
    listen 443 ssl http2;
    server_name your-domain.com;

    ssl_certificate     /path/to/fullchain.pem;
    ssl_certificate_key /path/to/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:5000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## 🛠️ 常用运维

```bash
# 查看日志
docker compose logs -f store

# 进入容器调试
docker compose exec store bash

# 重启
docker compose restart

# 升级到新版本
git pull && docker compose up -d --build

# 备份数据库（根据你 DB 所在机器调整）
mysqldump -u store_user -p nodeloc_store > backup_$(date +%F).sql

# 还原数据库
mysql -u store_user -p nodeloc_store < backup.sql
```

## 🔒 安全建议

- ✅ 生产环境建议使用 HTTPS（开发环境可勾选「允许 HTTP 回调」跳过）
- ✅ 数据库用户只授予 `nodeloc_store` 库的权限，不要用 root
- ✅ 修改 Admin 默认用户名
- ✅ 定期备份数据库与 `instance/config.ini`
- ✅ OAuth `email` scope 需在 NodeLoc 审核通过；未通过时用户的邮箱将为空
- ✅ 不要把 `instance/config.ini` 提交到 Git（已在 `.gitignore` 中）

## 📁 项目结构

```
nodeloc-store/
├── app/
│   ├── __init__.py          # Flask factory + 每次请求重读 config
│   ├── config.py            # 配置加载（instance/config.ini 实时）
│   ├── extensions.py        # db / login_manager / csrf
│   ├── models.py            # User / Product / Card / Order / AppSetting / AuditLog
│   ├── nodeloc.py           # NodeLoc OAuth2 + Payment 客户端
│   ├── utils.py             # PBKDF2 密码 / slug / audit
│   ├── blueprints/
│   │   ├── install.py       # 首次安装向导
│   │   ├── auth.py          # 邮箱 + NodeLoc OAuth 登录
│   │   ├── store.py         # 公开商店
│   │   ├── payment.py       # 支付 + 回调
│   │   ├── user.py          # 用户中心 + 绑定 OAuth
│   │   ├── admin.py         # 后台
│   │   └── api.py           # JSON API
│   └── templates/           # Jinja2 模板
├── Dockerfile
├── docker-compose.yml
├── requirements.txt
└── README.md
```

## 🐛 故障排查

| 问题 | 解决 |
|---|---|
| 向导卡在数据库步骤 | 检查 MySQL/MariaDB 是否启动、端口是否开放、用户是否有权限 |
| 数据库连接超时 | 容器与数据库不在同一网络？防火墙是否放行 `3306`？云数据库安全组？ |
| 回调签名验证失败 | 确认 `instance/config.ini` 中的 `payment.secret` 与 NodeLoc 一致 |
| OAuth 登录失败 | 检查回调地址是否与 NodeLoc OAuth 应用配置完全一致（含协议/HTTPS） |
| 邮件没拿到 | NodeLoc OAuth `email` scope 需审核通过；未通过时 token 只有 `openid` |
| 卡密一直没发货 | 检查 Admin → 日志 中 `payment.stock_warning` 条目，确认有可用卡密 |

## 📜 License

MIT
