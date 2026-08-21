# NodeLoc Store

> 在线卡密商店，支持 NodeLoc OAuth2 登录 + 积分支付，MariaDB 存储，Docker 部署。

## 功能

- 🔐 **多渠道登录**：NodeLoc OAuth2 一键登录 + 邮箱注册/登录
- 🔑 **卡密商品**：Admin 后台管理商品（名称/价格/图片/库存），批量导入卡密
- 💰 **NodeLoc 积分支付**：用户使用 NodeLoc 积分付款，自动回调发货
- 📦 **订单管理**：用户查看历史订单 / 管理员处理退款
- 🎨 **精美 UI**：玻璃拟态 + 渐变设计，深/浅色主题

## 快速开始

### Docker Compose（推荐）

```bash
cp .env.example .env
# 编辑 .env 填入数据库密码
docker compose up -d
# 访问 http://your-domain:5000 完成快速开始向导
```

**首次访问向导**会逐步引导你填写：

1. **数据库** — MariaDB 主机/端口/用户名/密码/库名
2. **商店信息** — 名称与标语
3. **NodeLoc OAuth** — URL / Client ID / Client Secret / Redirect URI / Scope
4. **NodeLoc 支付** — Payment ID / Secret Key
5. **管理员账号** — 首个管理员用户名与密码

> ⚠️ **提前在 NodeLoc 创建 OAuth 应用和支付应用**（需要 TL1 银会员及以上），把回调地址填为：
> - OAuth: `https://你的域名/auth/oauth/callback`
> - 支付: `https://你的域名/payment/callback`
>
> 如使用 `email` scope，需要管理员审核通过。

### 本地开发

```bash
python -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
flask --app run.py run --debug
```

## 环境变量 / 首次安装

首次访问会进入**快速开始向导**，填写：

1. **数据库**：MariaDB 主机/端口/用户名/密码/库名
2. **Admin 账号**：设置首个管理员用户名和密码

后续可在 **Admin 后台 → 设置** 填写：

| 字段 | 说明 |
|---|---|
| `OAuth URL` | `https://www.nodeloc.com` |
| `OAuth Client ID` | NodeLoc OAuth 应用 ID |
| `OAuth Client Secret` | NodeLoc OAuth 密钥 |
| `OAuth Redirect URI` | `https://你的域名/auth/oauth/callback` |
| `Payment ID` | NodeLoc 支付应用 ID |
| `Payment Secret` | NodeLoc 支付密钥 |

## 项目结构

```
app/
├── __init__.py       # Flask factory
├── config.py          # 配置加载（从 instance/config.ini）
├── extensions.py      # Flask 扩展（db, login_manager, csrf）
├── models.py          # SQLAlchemy 模型
├── nodeloc.py         # NodeLoc OAuth2 + Payment 客户端
├── utils.py           # 通用工具（密码 / slug / audit）
├── blueprints/
│   ├── admin.py       # Admin 后台
│   ├── api.py         # JSON API
│   ├── auth.py        # 登录/注册/OAuth
│   ├── install.py     # 首次安装向导
│   ├── payment.py     # 支付流程
│   ├── store.py       # 公开商店
│   └── user.py        # 用户中心
└── templates/         # Jinja2 模板
```

## License

MIT
