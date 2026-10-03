# 第二阶段：后台布局、工单中心与设置页整改审计

## 1. 项目审计结果

### 目录结构
- 后端 `internal/modules/{identity,payment,catalog,activity,support,notification,audit,plugin,system}`，
  每个模块分 `domain / contract / infrastructure / application / transport/http`。
- 管理端 24 个页面 + 6 个通用组件 + 15 个 API 模块；用户端 11 个页面 + 5 个组件 + 7 个 API 模块。

### 后台路由（29 条）
总览 / 商品（列表、新建、编辑）/ 卡密 / 订单（列表、详情）/ 用户（列表、详情）/
分类 / 优惠券 / 活动（列表、新建、详情、编辑）/ 客服中心 / 知识库 / 客服与快捷回复 /
配置中心 / 通知中心 / 插件 / 审计日志 / 系统设置 / 角色权限 / 无权限 / 404。

### 用户端路由（13 条）
首页 / 登录 / 注册 / 商品详情 / 订单（列表、详情）/ 个人中心 / 活动中心 /
客服中心 / 我的工单 / help→support 跳转 / OAuth 回调 / 404。

### 组件现状（改造前）
- 布局类：`SideBar`（侧栏）、`App.vue`（顶栏 + 面包屑 + 内容区）。
- 通用类：`PaginationFooter`、`StatCard`、`ImageField`、`AdminIcon`、`RouteProgress`、`SettingsSection`。
- **缺失**：统一的保存操作栏、抽屉、弹窗、表格组件（各页各写一份）。

### 请求封装
管理端 `api/client.ts`：axios 实例 + Bearer 注入 + 401 静默刷新 + 单飞刷新去重。
用户端同构。错误统一走 `errorMessage / errorStatus / errorCode`。

## 2. 后台 UI 改造说明

### 主布局
- 左侧可折叠侧栏（lg 断点以下为抽屉），按权限过滤菜单项。
- 顶部栏：页面标题、面包屑、主题切换、通知入口（未读角标）、返回前台、管理端身份。
- 内容区统一 `px-5 pb-12 pt-7`，页面切换使用 keyed transition。

### 首页重新设计
- 数据卡片：期间收入、支付订单、客单价、期间新客、AI 处理中工单、待人工处理、
  进行中活动、AI 工具调用，支持显示开关与拖动排序。
- **工单中心区块（本阶段新增）**：未读 / AI 处理中 / 待人工 / 即将超时四个可点卡片，
  最近工单列表（每条直达该工单），刷新按钮，错误重试。
- 快捷操作（本阶段新增）：新增商品、导入卡密、创建活动、全部订单、工单中心、
  AI 客服配置、知识库、系统设置，每一项都按权限显示。
- 经营数据：销售趋势、订单趋势、热销商品、分类销售、优惠码、卡密健康度、漏斗。

## 3. 菜单与路由变化

本阶段没有删除任何路由，只调整了侧栏与跳转目标：
- 侧栏保持 15 个一级入口（上一轮已把 4 个重复指向配置中心的入口合并）。
- 首页工单卡片链接改为携带筛选参数：
  `/service?attention=unread`、`/service?status=pending_human`、
  `/service?attention=overdue`、`/service?attention=urgent`。
- 待办事项的「期间退款」补上 `/orders?status=refunded`。

## 4. 工单中心按钮修复清单

| 按钮 | 修复前 | 修复后 |
| --- | --- | --- |
| 首页「待人工工单」 | 跳到 /service，无筛选 | `?status=pending_human` |
| 首页「即将超时工单」 | 有 attention=overdue，但页面不读该参数 | 页面读取并生效 |
| 首页「未读工单」 | 不存在 | 新增卡片，`?attention=unread` |
| 首页「紧急工单」 | 不存在 | 新增卡片，`?attention=urgent` |
| 首页工单列表项 | 不存在 | 新增，点击直达该工单 |
| 首页「刷新工单」 | 不存在 | 新增，重新请求接口 |
| 工单面板「转人工」 | 不存在 | 新增，AI 处理中的工单可手动转人工 |
| 工单面板「自动分配」 | 后端有接口，前端无入口 | 新增按钮 |

根因：`ServiceCenterView` 只读取 `route.query.attention`，不读 `status`/q/mine，
所以首页带 `status` 的跳转会落到全部工单。现在地址栏的
`attention / status / mine / q / handler` 全部作为筛选来源，并且切换筛选后
右侧面板会自动改选仍在列表中的工单。

## 5. 设置页保存问题修复清单

| 页面 | 问题 | 修复 |
| --- | --- | --- |
| AI 基础配置 | 保存按钮在上次改造中被移到页面头部，面板内无按钮 | 改用统一 SaveBar |
| AI 提示词 | 同上 | 同一条 SaveBar 按页签切换 |
| AI 工作流 | 同上 | 同一条 SaveBar 切换为「保存工作流」 |
| 客服与快捷回复 | 保存按钮藏在长表单中部 | 顶部新增 SaveBar |
| 知识库 | 保存按钮只在右侧栏，窄屏不易发现 | 顶部新增 SaveBar |

新增统一组件 `components/SaveBar.vue`：固定吸附在顶部，包含未保存状态、
保存、取消修改、恢复默认、测试配置按钮，只读角色显示为禁用态。
密钥类字段仍由后端处理：空值保留原密钥，`__clear__` 才清除，
读取接口不返回密钥，保存后重新拉取服务端数据。

## 6. 新增或修改的 API

- `GET /api/v1/admin/stats` 新增 `tickets_unread`、`tickets_urgent` 两个字段。
  这是唯一改动的接口，只加字段，不改结构，向后兼容。

## 7. 新增组件

- `frontend/admin/src/components/SaveBar.vue` —— 统一配置操作栏。

## 8. 权限变化

无。沿用现有 Casbin 资源，新增按钮都按既有权限显示/隐藏。

## 9. 数据库变更

无。没有新增表或列。

## 10. 已知问题

- 工单附件上传仍未接入前端控件（表结构与后端校验位已就绪）。
- 工单「转交其他客服」目前是下拉选择，未做独立的转交确认弹窗。

## 11. 最终验收结果

- `go build ./...` 通过；`go test ./...` 全部通过。
- 管理端与用户端 `vue-tsc` 与 `vite build` 通过；`smoke_test.py` 23 项通过。
- 真实服务端到端验证：工单按状态筛选（待人工 1 / AI 处理中 2）、
  按 attention 筛选（未读 3 / 紧急 0 / 超时 0）全部与服务端数据一致；
  首页统计返回 tickets_unread / tickets_urgent；
  AI 配置与工作流保存后重新读取仍是新值，密钥字段不回传。
