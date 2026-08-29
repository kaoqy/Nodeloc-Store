# NodeLoc Store v2.0

基于 Dujiao-Next 架构的数字商品发卡平台，Go + Vue 3 全栈实现。

## 架构

```
├── cmd/server/              # 入口
├── internal/
│   ├── app/
│   │   ├── container/       # 依赖注入容器
│   │   └── httpserver/      # JWT/RBAC 中间件
│   ├── authz/               # Casbin RBAC
│   ├── modules/
│   │   ├── identity/        # 用户认证 + NodeLoc OAuth2
│   │   ├── payment/         # 支付 + NodeLoc Payment
│   │   ├── catalog/         # 商品/卡密/分类/优惠券
│   │   ├── notification/    # 系统通知
│   │   └── audit/           # 审计日志
│   ├── architecture/        # 架构守护测试
│   ├── models/              # GORM 模型
│   ├── shared/              # 跨模块公共工具
│   └── platform/            # 数据库连接
└── frontend/
    ├── user/                # 用户商店 (Vue 3 + Vite + Tailwind)
    └── admin/               # 管理后台 (Vue 3 + Vite + Tailwind)
```

## 快速开始

```bash
# 后端
cp config.yml.example config.yml
# 编辑 config.yml 填入 NodeLoc OAuth + Payment 凭据
go run cmd/server/main.go

# 前端
cd frontend && pnpm install
pnpm dev:user    # http://localhost:5173
pnpm dev:admin   # http://localhost:5174
```

## Docker

```bash
docker build -t kaoqy666/nodeloc-store:latest .
docker run -d --name nodeloc-store -p 8080:8080 \
  -v $PWD/config.yml:/app/config.yml \
  kaoqy666/nodeloc-store:latest
```

## 测试

```bash
go test ./internal/architecture/...   # 架构守护
go test ./...                         # 全量测试
```

## License

MIT
