# NodeLoc Store v1.0.0 — 活动营销 / 工单 / AI 客服改造说明

本文件记录本次改造的交付内容。版本号保持 v1.0.0，tag 不变。

## 一、现有项目结构分析

- 后端：Go 1.26 + Gin + GORM（SQLite / MySQL），模块位于 `internal/modules/<模块>`，
  每个模块按 `domain / contract / infrastructure / application / transport/http` 分层，
  模块之间只能通过 `contract` 或 `domain` 引用，由 `internal/architecture` 测试守住。
- 数据模型：集中在 `internal/models`，`Migrate()` 使用 GORM AutoMigrate 建表。
  改造前已有 User / OAuth / Point / Checkin / Category / Product / Card / Order /
  DeliveryRecord / Coupon / Plugin / PluginBinding / Notification / AuditLog / AppSetting。
- 权限：Casbin，`resource:action` 两段式，后台接口统一用
  `middleware.RequirePermission` 二次读取账号状态后判定。
- 支付：`internal/modules/payment` 自洽，下单金额在服务端算出后交给 NodeLoc Payments；
  本次没有改动支付核心逻辑，只增加了活动定价端口与订单活动快照。
- 前端：管理端为用户端各一套 Vue 3 + Vite + Tailwind 4 应用，设计语言为
  `--brand` 驱动的 “Ember on Ink” 双主题，本次沿用同一套 token 与组件习惯。

## 二、后台信息架构（改造前 → 改造后）

改造前：概览 / 商品 / 卡密 / 订单 / 用户 / 分类 / 优惠券 / 插件 / 通知 / 角色 / 日志 / 设置。

改造后侧边栏分组：

| 分组 | 页面 |
| --- | --- |
| 概览 | 仪表盘 |
| 经营 | 商品管理、订单管理、卡密管理、分类管理、优惠券、**活动营销**、插件管理 |
| 客服与 AI | **工单中心**、**AI 客服**、**AI 工具**、**知识库**、**客服与快捷回复** |
| 客户 | 用户管理 |
| 系统 | 通知中心、角色权限、审计日志、系统设置 |

## 三、新增数据库表

活动：`activities`、`activity_rules`、`activity_records`、`activity_logs`、`coupon_records`。
订单新增列：`activity_id`、`activity_name`、`activity_discount`、`activity_snapshot`。
工单：`tickets`、`ticket_messages`、`ticket_logs`、`ticket_attachments`、`ticket_ai_sessions`。
AI：`ai_conversations`、`ai_messages`、`ai_tool_definitions`、`ai_tool_permissions`、
`ai_tool_calls`、`ai_knowledge`、`ai_knowledge_categories`、`ai_feedbacks`、`ai_configs`、
`ai_workflow_configs`、`ai_quick_questions`。
客服：`customer_service_agents`、`customer_service_assignments`、`quick_replies`。
通知与配置：`notification_templates`、`notification_logs`、`system_configs`、`operation_logs`。

索引与完整 DDL 见 `scripts/migrations/2026_activities_tickets_ai.sql`；
生产环境由启动时的 AutoMigrate 建表，脚本用于审计与手工执行。

## 四、新增 API

活动（后台）：`GET/POST /api/v1/admin/activities`、
`GET/PUT/DELETE /api/v1/admin/activities/:id`、
`POST /api/v1/admin/activities/:id/duplicate`、
`POST /api/v1/admin/activities/:id/status`、
`GET /api/v1/admin/activities/:id/stats|records|logs`、
`GET /api/v1/admin/activity-overview`、`GET /api/v1/admin/coupon-records`。
活动（买家）：`GET /api/v1/store/activities`、`GET /api/v1/store/activities/:id`、
`POST /api/v1/activities/:id/claim`、`GET /api/v1/me/activity-records`。

工单（买家）：`GET/POST /api/v1/me/tickets`、`GET /api/v1/me/tickets/:id`、
`POST /api/v1/me/tickets/:id/messages|transfer|read|rate|reopen|cancel`。
工单（后台）：`GET /api/v1/admin/tickets`、`GET /api/v1/admin/ticket-stats`、
`GET /api/v1/admin/tickets/:id`、`POST /api/v1/admin/tickets/:id/messages|status|assign|auto-assign|transfer|read`、
`GET /api/v1/admin/tickets/:id/summary`。

AI：`GET/POST /api/v1/support/config|quick-questions|chat|conversations/:id|feedback`；
`GET/PUT /api/v1/admin/ai/config`、`PUT /api/v1/admin/ai/workflow`、
`GET /api/v1/admin/ai/conversations|feedback`、
`GET /api/v1/admin/ai/quick-questions`（另有 PUT/DELETE :id）。
AI 工具：`GET /api/v1/admin/ai-tool-index`、`GET /api/v1/admin/ai-tool-index/calls|roles`、
`PUT /api/v1/admin/ai-tools/:id`、`POST /api/v1/admin/ai-tools/:id/permissions`。
知识库：`GET /api/v1/admin/knowledge-index`（列表）、`POST /api/v1/admin/knowledge-index`（新建）、
`GET/PUT/DELETE /api/v1/admin/knowledge/:id`、`POST /api/v1/admin/knowledge/:id/test`、
`GET/POST/DELETE /api/v1/admin/knowledge-index/categories`、`POST /api/v1/admin/knowledge-index/import`、
`GET /api/v1/admin/knowledge-index/export`。
客服：`GET/POST/PUT/DELETE /api/v1/admin/agents`、
`GET/POST/PUT/DELETE /api/v1/admin/quick-replies`、`POST /api/v1/admin/quick-replies/:id/render`。

## 五、前端页面与路由

管理端新增：`/activities`、`/activities/new`、`/activities/:id`、`/activities/:id/edit`、
`/tickets`、`/tickets/:id`、`/ai`、`/ai/tools`、`/knowledge`、`/service/agents`。
用户端新增：`/activities`（活动中心）、`/tickets`（我的工单），
以及全局悬浮的 AI 客服窗口（首页、商品页、订单页、移动端都能打开）。

## 六、AI 工具与权限

内置工具（`internal/modules/support/application/tools.go`）：
`user.profile`、`order.list`、`order.detail`、`order.payment_status`、`order.delivery_status`、
`product.detail`、`product.list`、`activity.list`、`knowledge.search`、`ticket.list`、
`ticket.create`、`ticket.transfer`、`ticket.add_message`、`ticket.update_status`、
`notification.send`、`coupon.grant`。

安全约束：未登记的工具直接拒绝；工具必须启用；需要登录的工具校验用户身份；
参数走白名单与类型校验；订单类工具在适配器层校验归属，越权返回“查不到”；
调用前检查用户角色授权与每分钟限频；每个工具有独立超时；
参数与结果写入 `ai_tool_calls` 前都会脱敏；高风险工具默认停用并要求二次确认。

## 七、工单流转与转人工

创建工单 → 状态 `ai_processing`，AI 读知识库并在授权范围内调用工具 →
有结论则转为 `waiting_user` 等待用户确认 → 用户继续追问则重复上述过程 →
用户主动要求人工、AI 连续失败、点踩达到阈值、或命中退款/卡密/支付/封禁敏感策略时 →
生成问题摘要与推荐处理方案 → 状态转为 `pending_human` →
按客服在线状态与并发量自动分配 → `human_handling` → `waiting_confirm` / `resolved` / `closed` → 用户评价。

转人工保留完整 AI 对话：工单消息表不区分 AI 与人工，转人工只改状态与处理方，
客服在工单工作台能看到全部消息、工具调用记录和操作时间线。

## 八、权限角色

保留原有 `super_admin / admin / operator / support`，新增
`ops_manager / product_manager / order_manager / finance / support_lead / support_agent / ai_admin / data_viewer`。
新增权限资源：`activities`、`tickets`、`ticket_config`、`ai`、`ai_tools`、`knowledge`、
`agents`、`quick_replies`、`notification_templates`、`config_center`、`operation_logs`、`finance`。
既有店铺通过 authz 种子版本 5/6 自动补齐新菜单的查看权限。

## 九、环境变量与外部服务

- AI 服务：`AI_BASE_URL` 可在配置页填写任意 OpenAI 兼容地址；API Key 使用
  AES-256-GCM 加密后存 `ai_configs.api_key_enc`，读接口只返回是否已配置。
- SMTP：沿用原有 `SMTP_*` 环境变量与后台设置，AI 通知复用同一套邮件发送实现。
- 环境变量说明：本次没有新增必需的环境变量；数据库、JWT、NodeLoc OAuth 与
  NodeLoc Payments 沿用原有配置。

## 十、操作日志

- 活动：`activity_logs` 记录创建、更新、上下架、暂停、删除，含前后快照与操作者。
- 工单：`ticket_logs` 记录状态、负责人、优先级、转人工与消息事件。
- AI：`ai_tool_calls` 记录用户、会话、工具、参数（脱敏）、结果（脱敏）、耗时与结果状态。
- 通用：`operation_logs` 提供跨模块审计结构；后台所有写操作仍经过
  `middleware.AdminAudit` 写入 `audit_logs`。

## 十一、验证结果

- `go build ./...` 通过。
- `go test ./...` 全部通过（含路由注册测试与架构分层测试）。
- 管理端 `vue-tsc --noEmit` 通过，`vite build` 通过。
- 用户端 `vue-tsc --noEmit` 通过，`vite build` 通过。

## 十二、已知问题

- AI 的实际回答依赖店铺配置的模型服务；未配置时 AI 客服窗口自动隐藏，
  工单落在待人工队列，这是设计上的降级而不是故障。
- 工单附件表已建好，当前只提供上传前的类型与大小校验位置，尚未接入前端上传控件。
- `system_configs` / `operation_logs` 为后续配置中心预留，当前读取写入仍以
  现有 `app_settings` 与 `audit_logs` 为主。

## 十三、后续扩展建议

- 为工单附件补齐上传与图片识别。
- 把 `system_configs` 接成统一配置中心页面，逐步迁移散落的设置。
- 给 AI 结果增加引用来源展示与命中率统计。
- 活动规则增加 AB 测试与自动效果对比报表。


---

# 第二轮调整（首页分流 / 后台配置中心整合）

## 用户端

- 首页新增独立的「正在进行的优惠」区块，只讲促销；AI 客服仍然是右下角悬浮窗，
  两个入口不再混在一起，买家能分清「活动」和「找客服」。
- 新增「客服中心」页面（`/support`），把帮助文档、我的工单、活动中心收进一个入口；
  导航栏与页脚统一显示「客服中心」。
- 帮助中心不再作为独立菜单存在；`/help` 保留为跳转到客服中心，
  已发出的链接不会失效。原 HelpView 与其登录后才显示的菜单项已移除。
- 客服中心的「打开智能客服」按钮通过事件直接唤起悬浮窗，
  买家用一个入口就能找到对话框，不会再看到两套聊天界面。

## 后台

- 工单中心与 AI 客服合并为一级入口「客服中心」（`/service`）：
  左侧工单队列 + 右侧处理面板，AI 工具调用、内部备注、快捷回复、转交客服都在同一屏。
- 工单处理面板从原详情页拆成 `views/support/TicketDetailPanel.vue`，
  旧的 `TicketListView` / `TicketDetailView` 已移除。
- AI 与客服的全部配置集中进「配置中心」，左侧分组导航：
  AI 基础与工作流、AI 工具与权限、知识库、客服与快捷回复、工单配置、
  通知模板（含发送日志）、风控配置、数据保留、文件上传。
- `/knowledge` 与 `/service/agents` 保留为配置中心对应分组的直达地址，
  侧栏也保留这两个快捷入口。
- 总览新增快捷入口卡片，待办事项补齐「待人工工单 / 即将超时工单 / 自动发货异常」，
  并可从待办直接跳到客服中心的对应筛选。

## 配置真正生效

- 新增 `SystemConfigValue / SystemConfigInt / SystemConfigBool` 读取器，
  配置中心保存的值立刻影响运行行为，不需要重启。
- 工单编号前缀（`ticket_no_prefix`）与人工响应超时（`human_timeout_hours`）
  在创建工单时读取。
- `allow_reopen`、`allow_cancel`、`enable_rating` 分别控制
  重新打开、撤销工单与提交评价三个接口，后台关掉即拒绝。

## 验证

- `go build ./...` 通过；`go test ./...` 全部通过（新增配置读取器三项测试）。
- 管理端与用户端 `vue-tsc` 与 `vite build` 通过。
- `scripts/smoke_test.py` 23 项通过。

## 说明

版本号与 tag 在这一轮保持不变（仍为 v1.0.0）。
