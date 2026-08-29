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
- 🔒 **安全** — PBKDF2 密码哈希、CSRF 全部 POST、回调 HMAC 验签、操作审计日志

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
- Python 3.10+ + pip + venv
- MySQL 5.7+ / MariaDB 10.3+（本机或远程均可）
- 已配好 OpenResty（含 SSL，Let's Encrypt 推荐）
- NodeLoc 论坛账号：**白银会员 TL1**及以上才能创建支付应用；**OAuth 需 TL2 黄金会员**

### Step 1 · 准备数据库

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

如果是远程 DB，把 `'store_user'@'localhost'` 改为 `'store_user'@'%'`，并确保数据库服务器 `bind-address` 与防火墙允许应用服务器连接。

### Step 2 · 在 NodeLoc 创建应用

> NodeLoc 创建应用时填的回调地址，**必须**是你 OpenResty 反代出来的 HTTPS 域名（不能是 `http://127.0.0.1:5000`）。

#### 2.1 OAuth 应用

访问 <https://www.nodeloc.com/oauth-provider/applications> → 创建应用：

| 字段 | 填什么 |
|---|---|
| 应用名称 | 你的商店名 |
| 网站地址 | `https://你的域名` |
| 回调地址 | `https://你的域名/auth/oauth/callback` |
| 权限范围 | 勾选 `openid`（必选）、`profile`、`email`（需审核） |

保存后记录 **Client ID** 和 **Client Secret**（只显示一次）。

#### 2.2 支付应用

访问 <https://www.nodeloc.com/payment/applications> → 创建应用：

| 字段 | 填什么 |
|---|---|
| 应用名称 | 你的商店名 |
| 网站地址 | `https://你的域名` |
| 回调地址 | `https://你的域名/payment/callback` |

保存后记录 **Payment ID** 和 **Secret Key**（只显示一次）。

### Step 3 · 启动商店（Docker）

推荐直接使用 Docker Hub 已发布的多架构镜像一键部署：

```bash
docker run -d --name nodeloc-store --restart unless-stopped \
  --network 1panel-network \
  -p 5000:5000 \
  -v "$PWD/instance:/app/instance" \
  -v "$PWD/uploads:/app/uploads" \
  kaoqy666/nodeloc-store:latest
```

固定使用 `1.0.0` 版本：

```bash
docker run -d --name nodeloc-store --restart unless-stopped \
  --network 1panel-network \
  -p 5000:5000 \
  -v "$PWD/instance:/app/instance" \
  -v "$PWD/uploads:/app/uploads" \
  kaoqy666/nodeloc-store:1.0.0
```

> `1Panel-mariadb-oxyE` 这类容器名只有在同一个 Docker 网络中才能解析。上面的 `--network 1panel-network` 会让商店连接到 1Panel 数据库网络。如果实际网络名不同，先运行 `docker inspect 1Panel-mariadb-oxyE --format '{{range $name, $_ := .NetworkSettings.Networks}}{{$name}}{{end}}'` 查询，然后替换命令中的网络名。命令同时会把配置文件持久化到当前目录的 `instance`，把商品图片持久化到当前目录的 `uploads`。

如果需要从源码构建，也可以使用 Docker Compose：

```bash
git clone https://github.com/kaoqy/Nodeloc-Store.git
cd Nodeloc-Store
DB_NETWORK=1panel-network docker compose up -d
```

> Compose 只起一个 `store` 容器，并把它加入 `DB_NETWORK` 指定的外部网络。MariaDB 使用现有的 1Panel 实例，连接信息在安装向导中填写。若 1Panel 的数据库网络不是 `1panel-network`，请将 `DB_NETWORK` 改为通过 `docker inspect` 查到的名称。

查看日志：
```bash
docker logs -f nodeloc-store
```

使用 Docker Compose 部署时，查看日志的命令为 `docker compose logs -f store`。

### Step 4 · OpenResty 反代 + SSL

在 OpenResty 配置目录加一个 server 块（路径以你的环境为准，比如 `/usr/local/openresty/nginx/conf/conf.d/store.conf`）：

```nginx
server {
    listen 443 ssl http2;
    server_name your-domain.com;

    ssl_certificate     /path/to/fullchain.pem;
    ssl_certificate_key /path/to/privkey.pem;

    client_max_body_size 8M;

    # 静态资源直接走 nginx
    location /static/ {
        alias /opt/Nodeloc-Store/app/static/;
        expires 7d;
        access_log off;
    }
    location /admin/uploads/ {
        alias /opt/Nodeloc-Store/uploads/products/;
        expires 7d;
        access_log off;
    }

    # 反代到 Docker 容器里 gunicorn 监听的 5000
    location / {
        proxy_pass http://127.0.0.1:5000;
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

> `alias` 路径要跟宿主机上 `/opt/Nodeloc-Store/` 实际路径一致。容器卷默认 `./instance:/app/instance` 和 `./uploads:/app/uploads`，所以 `uploads/products` 对应宿主机的 `./uploads/products`。

### Step 5 · 完成快速开始向导

浏览器访问 `https://你的域名` 进入 5 步安装向导：

1. **数据库配置** — 填 Step 1 创建的连接信息
   - 主机：`localhost`（或远程 IP）
   - 端口：`3306`
   - 库名/用户/密码：同 Step 1
2. **商店信息** — 名称与标语
3. **NodeLoc OAuth** — 填入 Step 2.1 的 Client ID / Secret / Redirect URI / Scope
4. **NodeLoc 支付** — 填入 Step 2.2 的 Payment ID / Secret Key
5. **管理员账号** — 创建首个管理员

> 提交时如果数据库连不通，会显示错误提示让你重填，**不会破坏配置**。
> **保存即生效**，不用重启容器。

### Step 6 · 验证支付

1. 用 Admin 账号登录后台 → **商品** → 新建一个商品 + 导入几个测试卡密
2. 退出登录，用另一个 NodeLoc 账号或邮箱注册普通用户
3. 下单购买 → 跳转到 NodeLoc 支付页 → 用积分支付
4. 支付完成后浏览器会跳转回你的 `/payment/callback`，自动发货，卡密会显示在订单详情

## 🛠️ 常用运维

```bash
# 看容器日志
docker compose logs -f store

# 重启容器
docker compose restart store

# 升级到新版本
cd /opt/Nodeloc-Store   # 或你的实际路径
git pull
docker compose up -d --build

# 备份数据库（MariaDB 装在宿主机或远程时调整连接信息）
mysqldump -u store_user -p nodeloc_store > backup_$(date +%F).sql

# 还原数据库
mysql -u store_user -p nodeloc_store < backup.sql

# 健康检查（OpenResty upstream 用）
curl -I http://127.0.0.1:5000/api/health
# {"status":"ok","installed":true,"db":"ok"}
```

## 🔒 安全建议

- ✅ OpenResty 必须配置 SSL，NodeLoc 强制要求回调为 HTTPS
- ✅ DB 用户只授予 `nodeloc_store` 库的权限，不要用 root
- ✅ 修改 Admin 默认用户名
- ✅ 定期备份数据库与 `instance/config.ini`
- ✅ OAuth `email` scope 需在 NodeLoc 审核通过；未通过时用户的邮箱将为空
- ✅ 不要把 `instance/config.ini` 提交到 Git（已在 `.gitignore` 中）

## 🧪 自测（不依赖任何第三方包）

```bash
python3 scripts/smoke_test.py
# PASS: 23    FAIL: 0
```

## 📁 项目结构

```
nodeloc-store/
├── app/                      # Flask 应用
│   ├── __init__.py          # Flask factory + 每次请求重读 config
│   ├── config.py            # 配置加载（instance/config.ini 实时）
│   ├── extensions.py        # db / login_manager / csrf
│   ├── models.py            # User / Product / Card / Order / AppSetting / AuditLog
│   ├── nodeloc.py           # NodeLoc OAuth2 + Payment 客户端
│   ├── utils.py             # PBKDF2 密码 / slug / audit
│   ├── blueprints/          # 路由
│   │   ├── install.py       # 首次安装向导
│   │   ├── auth.py          # 邮箱 + NodeLoc OAuth 登录
│   │   ├── store.py         # 公开商店
│   │   ├── payment.py       # 支付 + 回调
│   │   ├── user.py          # 用户中心 + 绑定 OAuth
│   │   ├── admin.py         # 后台
│   │   └── api.py           # JSON API + /health
│   ├── static/css/app.css   # 自带 CSS（无 Tailwind CDN 依赖）
│   └── templates/           # Jinja2 模板
├── scripts/smoke_test.py    # 23 个 stdlib 单元测试
├── Dockerfile
├── docker-compose.yml
├── requirements.txt
├── run.py                   # gunicorn 入口
└── README.md
```

## 🐛 故障排查

| 问题 | 解决 |
|---|---|
| 向导卡在数据库步骤 | MariaDB 启动了？端口开放？用户对库有权限？`mysql -u store_user -p nodeloc_store` 测一下 |
| 回调签名验证失败 | 确认 `instance/config.ini` 中的 `payment.secret` 与 NodeLoc 一致 |
| OAuth 登录失败 | 回调地址与 NodeLoc 应用配置完全一致（含 `https://`） |
| 邮件没拿到 | NodeLoc OAuth `email` scope 需审核通过；未通过时 token 只有 `openid` |
| 卡密一直没发货 | Admin → 日志 中 `payment.stock_warning` 条目，确认有可用卡密 |
| 静态资源 404 | 检查 OpenResty 的 `location /static/` 路径是否对应 `app/static/` |
| 上传图片 413 | OpenResty 的 `client_max_body_size` 与应用一致（默认 8M） |

## 📜 License

MIT

