# NodeLoc Store

> 参考 Dujiao-Next 运营与交付模式，基于 NodeLoc OAuth2 + Nodeloc Payments 的数字商品商店 · v1.0.0

一个完整可商用的数字商品与自动发卡平台：保留 **NodeLoc OAuth2 + 邮箱注册登录**，支付统一接入 **Nodeloc Payments**，支持卡密自动交付、人工交付、订单履约追踪、角色权限、运营设置与审计。当前衍生版本统一定义为 **v1.0.0**。

![license](https://img.shields.io/github/license/kaoqy/Nodeloc-Store)

## 🧩 插件标准（v1.0.0）

插件运行在这台服务器里，不下载、不执行第三方代码：一个插件就是「本版本内置的提供者」+「本店的启用与配置」。后续接入新插件只需实现一个接口并在 `internal/modules/plugin/wire.go` 注册，其余代码不用动。

**一套标准包含这几件事**

| 概念 | 位置 | 作用 |
| --- | --- | --- |
| `Plugin` | `internal/models/models.go` | 按 `key` 登记一个提供者，带开关、非敏感 `settings`、**只写不回读**的 `secrets`、配置表单 `config_schema` 与 `capabilities` |
| `PluginBinding` | 同上 | 把「商品 + 购买表单某个字段的取值」映射到提供方侧的交付项目 `remote_ref` |
| `contract.Provider` | `internal/modules/plugin/contract` | 提供者要实现的方法：`Key / Manifest / Validate / Deliver` |
| `capabilities` | `form` / `fulfill` / `notify` | 声明这个插件能贡献什么，后台与前台据此决定显示什么 |

**买家侧的一次购买**

1. 商品详情页渲染商品自己的购买表单；插件告诉前台这个商品要不要选择交付项目（`GET /api/v1/products/:id/plugin`）。
2. 点「立即购买」时服务端先做 `ValidateSelection`：选不到交付项就**当场拒绝**，不会让买家付了钱才卡住。
3. 付款确认后走 `Owns → Fulfill`：先把订单标成 `plugin_pending`，再交给插件。交付成功写回订单与交付记录；崩在中间时后台重试**仍然走插件**，不会退回卡密队列重复发一份。
4. 买家在订单详情页看到交付内容；`plugin_pending` 会显示「插件交付中」，交付完成后自动刷新。

**匹配规则怎么定**

- 匹配字段（`match_field`）指向购买表单项的 `key`；留空表示这个商品固定交付下面那一项。
- 匹配值统一 **去首尾空格 + 转小写** 后精确比对，所以 `Beijing`、` beijing ` 都能命中同一条规则。
- 没有命中时给的是「可选：…」这样的可执行提示，而不是随便发一个看起来差不多的东西。

**内置的参考实现**

「人工交付（按选项匹配）」(`manual-delivery-v1`，见 `internal/modules/plugin/infrastructure/manual_delivery.go`)：把买家选的选项匹配到店家写好的交付项目，付款后进入人工发货队列，并把这单匹配到哪一项写进交付内容。它把整套标准跑通，也是接真实 API 前的模板。

**站外提醒（SMTP）**

站内通知写成功后，如果店家在「系统设置」里开了 SMTP 且该账号有邮箱，同一封内容会作为邮件再发一次。邮件是尽力而为：站内那条已经落库，SMTP 挂了只写服务器日志，绝不会把一条成功的通知变成失败。主题带店铺名，正文把相对链接补成站点域名下的绝对地址，端口除 465/587 外也接受 25/2525（加密方式在设置里选）。

**密钥处理**

配置表单里的密码字段只写不回读：留空表示保持原样，输入新值才覆盖，发 `__clear__` 表示删除。任何读取接口都不会把凭据带出来，后台只会告诉你「这个字段已保存」。

---

## ✨ 功能特性

> SMTP 说明：当前 Go/Vue 主应用提供管理员受保护的 SMTP 测试发信接口，配置从环境变量读取；订单、验证码、找回密码等邮件事件尚未接通。用户邮箱用于账号资料/NodeLoc profile，站内通知仍通过应用内收件箱完成。不要把客服邮箱或站内通知描述成 SMTP。

- 🚀 **首次访问即安装** — 引导式配置数据库 + Admin 账号 + NodeLoc OAuth + 支付凭据
- 🔐 **双通道登录** — NodeLoc OAuth2 一键登录 / 邮箱注册登录，Scope 感知（`email` 未授权时自动隐藏）
- 💎 **精美 UI** — Tailwind + 玻璃拟态 + 渐变设计，深色主题，响应式
- 📦 **商品管理** — 卡密商品与人工交付商品、图片、定价、交付说明、联系方式要求、库存可见性和上下架；「推荐」标记进首页店长推荐位（列表页一键切换，商品表单同一字段），销量计数，列表支持搜索、排序（销量/价格/最新）与分页
- 🎫 **卡密系统** — 批量导入（每行一个，允许重复：同一个激活码卖给所有人都照常入库）、内置生成器、按商品与状态筛选、关键字搜索、批量启用/停用、批量删除、CSV 导出，库存自动同步
- 🧩 **插件** — 统一的交付标准：安装 / 配置 / 启用停用 / 移除，按「商品的某个购买表单项」把买家选的选项映射到具体交付项目；选不到就在付款前当场拒绝，付款后交付失败重试仍走插件，不会重复发一份。密钥只写不回读。—— 详见上面「插件标准」
- 💰 **Nodeloc Payments** — 所有订单统一走 Nodeloc Payments，支持多种回调参数、HMAC-SHA256 验签和幂等履约
- 💸 **店家直接转账 NL 给买家** — 后台「用户」列表每一行都有「转账」，不用离开列表就能把 NL 从商店自己的 NodeLoc 应用打到那位买家的账户（用户详情页有同一个入口和 TA 的历史流水）。金额限 1–1000 的整数，本地先拦住，不消耗 NodeLoc 的额度也不留流水；收款人必须已经用 NodeLoc 登录绑定过，没绑定的会直接说「这个账号还没有绑定 NodeLoc，请让对方先用 NodeLoc 登录一次」。成功与失败都进流水（`status` 为 `succeeded` / `failed`），转成功才给买家发站内通知，带上金额、流水号与店家留言。NodeLoc 的八种拒绝（余额不足 / 转账功能未开启 / 收款方对不上 / 不能给自己转 / 低于或超过限额 / 签名无效 / 单号已用过）各自翻成中文，并把对方报出来的数字留在文案里（需要多少、现有多少、限额多少），后面附 NodeLoc 原文。状态含糊（`pending`）或网关没有回音的一律按未完成处理，流水留着待查并写明「不要立刻重转」，不与真的拒绝混为一谈。列表右上「转账流水」看全店，用户详情页看单个人；每一次转账都写审计日志，操作者取自会话而不是请求体
- 🩺 **「无法支付」不再是一句死胡同** — 下单被服务商拒绝时，这次失败会记在该订单的支付流水上（一旦成功即清除），并且分清是网关暂时不通（提示稍后再点一次通常就好）还是店里参数没配好（明说重复点击不会有不同结果，请把订单号发给店家）。已经创建但没付成的订单不会变成孤儿：失败面板直接写出这一单的编号，「再试一次支付」用原单重发而不重复下第二单，另有「或去订单 XXXX 继续支付」的入口。后台设置新增「支付 API 地址」：留空时下单/查单/转账跟随 OAuth 域名，登录走镜像域而支付走主域的店必须填对，否则每一单都只会看到「无法支付」。设置接口同时回报 `payment_ready`、`payment_missing`（缺哪几项，用设置页上看得见的字段名点名）与 `payment_warnings`（填了但不可能对的凭据：Token 与 Secret Key 互换、把 OAuth 的 Client ID 填进 Payment ID、密钥还是 `********` 占位符）。「测试支付网关」不再把服务商的返回糊成一句结论，而是按 NodeLoc 实际回的东西分别说：参数没配齐（点名缺哪一项）/ 网关连不上 / 这个域上没有这个 Payment ID（`payment_id_unknown`，点名去支付应用页复制 `pay_` 那串）/ 查单只认浏览器会话（`provider_guarded`，判为**可达**并说明改用下单回执与回调核实）/ 时间戳被拒（`provider_clock`，直说去同步 NTP，密钥没问题）/ 签名被拒（`provider_rejected`，附商店依次试过的签名档）/ 签名已被接受。结论后面还会带上「本次实测：<这一路由真正生效的签名方式>」，店家看得见商店究竟在用哪把密钥签名。
- 🚚 **统一履约** — 卡密自动发货、缺货等待补货、人工交付、交付内容与备注、用户侧履约状态查询
- 📦 **补货即发货** — 店家导入、生成或重新启用卡密的那一刻，这件商品「已付款、正在等补货」的订单就按付款先后当场放出交付（一次调用最多放 100 笔），不用再干等 3 分钟一轮的后台清扫。接口回一个 `released` 数字，卡密页的成功提示直接念出「已自动发出 N 笔等待中的订单」，低库存面板与概览的库存预警也各带上「N 笔已付款在等」并按欠单多少排序——先补收了钱的那一格，而不是先补看起来最空的。只有真的上了货架的可用卡才算补货：单张新增即停用、批量停用都不谎报发货；别的商品的队列、未付款的订单都不受影响，已经交付过的订单不会被重复发卡。放队失败只写服务器日志，导入本身照样算成功（卡已经在架子上），后台清扫仍是兜底
- 📬 **缺货预警会主动找你** — 概览的「库存预警」卡片上多一个「提醒补货」：点一下就把此刻真的欠着的货架写进每一位补货人的站内通知，不必干等 3 分钟一轮的后台清扫（清扫照旧会跑同一轮，两者用的是同一套判断）。只有卖过或在等人手上的那几件会开口——从没卖出过一张的货架低于阈值也不吭声，免得常年刷屏。文案分得清现场：「已有 N 笔已付款的订单在等这一件商品，导入卡密后会立刻自动发货」「货架已经空了，下一笔付款会停在等待补货」「还剩 N 张，卖完就要手工补货」，并带上「补货阈值 T 张」，点「去处理 →」直达那一格卡密页。收件人是「打得开卡密页的人」：账号启用中、角色在权限矩阵里能看 `cards:view`（超级管理员、管理员、运营算，客服与普通买家不会被打扰，停用的账号也不算）。同一件商品对同一个人每个自然日只说一次（按商店本地时区算），所以第二遍点击接口回 `sent: 0` 而 `checked` 不变，页面念「今天已经提醒过 N 件缺货商品了，明天同一时间会再说一次」，而不是把同一句再发一遍。后台通知中心跟着一起有：侧栏「通知」上的未读角标、类别页签多出「库存预警」，一键「全部标为已读」把它们一起清掉
- 🏷️ **促销与优惠码** — 百分比/固定金额两种码，全场/指定商品/指定分类三种范围，生效窗口、最低消费、总量限用、每人限用；下单前可先试算，折扣直接参与 NodeLoc 收款金额
- 📣 **促销自己会说话** — 后台每张码多了一个「在前台展示」开关：默认不展示，私下发给某个客户的码不会被顺手公开；勾上之后商品详情页下方自动列出**此刻真的能用**的那些促销（还没到生效时间、已经过期、店家停用的都不会出现，限量码的剩余额度按真实占用算，含还没付款的订单，所以不会被前一个人占穿后还挂着）。每条写出立减多少、满多少可用、每人限用几次、有效期到什么时候、还剩几次，限定商品或分类的只在对应的页面上露面，点「用这个」当场填码试算，看到减免后的应付金额再下单；店家在设置里关掉优惠码功能，前台整块跟着收起。订单页与后台订单详情把「商品小计 → 优惠码减免（带上用的哪个码） → 本单实付」摊开写清，买家和店家看的是同一笔账
- 👥 **角色权限** — 超级管理员、管理员、运营、客服和普通用户五级角色，后台按 11 项资源 × 查看/管理逐项授权，权限矩阵由服务端目录驱动，每个角色标注当前账号数，越权的按钮不会出现在页面上
- 🎁 **用户运营** — 每日签到、连续签到奖励、积分流水、站点公告与客服信息；个人中心可以改昵称/简介/联系邮箱、上传自己的头像、从 NodeLoc 同步头像、绑定或换绑 NodeLoc 身份。站内通知在顶栏「个人中心」和头像上带未读角标（数的是整个收件箱而不是当前这一页，超过 99 条写成 `99+`），通知区一键「全部标为已读」会回报实际改了几条，而不是空喊成功
- 🔔 **订单动态自动通知** — 买家不必守着订单页刷新：卡密到账、人工单等待发货、缺货等待补货、商家已发货、退款回到 NodeLoc 账户，这五种时刻都会自动写进买家的站内通知（顶栏角标跟着亮起来），文案带上订单号和商品名，退款那条念的是「订单 XXXX 的 180 NL 已退回你的 NodeLoc 账户」——退回的就是当初扣走的那个整数，不会写成人民币，点开就是那一单的详情页。同一笔订单重复对账、重复发货不会把同一条消息发第二遍；通知写不进去只记服务器日志，绝不会把已经收到的钱显示成支付失败
- 🗂️ **通知中心分门别类** — 收件箱不再是一条越滚越长的列表：通知区上面一排标签把消息按类别分开（全部、未读、订单、促销、系统，本店真有一条该类消息时才出现该类），每个标签上写着这一类的总数，未读的那些另外标出「· N 未读」。点「未读」只看还没读过的，点某一类只看那一类，两个条件可以叠着一起用；筛完这一页没有消息时不会只甩一句「暂无通知」，而是说明是「这一类没有未读的了」还是「这个类别还没有通知」，并给一个「看全部」把筛选放回去。一屏装十条，底部「加载更多（还有 N 条）」往下续，N 是剩下的确切条数；标完已读后各类标签上的未读数就地减一，不用整页重拉。清空后所有标签的未读标记一起消失，顶栏头像上的角标也一起落下
- 📦 **订单也能整批带走** — 订单页有「导出订单 CSV」，导出的就是屏幕上那一批：状态、关键词、指定买家、「需处理交付」哪个筛选在生效，下载的文件里就是哪个范围，一笔不多一笔不少（服务端在响应头里报实际行数，前端照实念出「已导出当前筛选的 72 笔订单」，不让人自己数行）。列带单号、状态与交付状态、商品与数量、单价(NL)/优惠(NL)/实付(NL)（就是店家定的、屏幕上显示、向 NodeLoc 申请收款的那个数：列名带单位，整数照抄，不补两位小数也不做任何换算）、优惠码、买家 ID/用户名/邮箱、联系方式、NodeLoc 交易号、发出卡密的条数、下单/支付/交付时间。文件开头带 UTF-8 BOM，Excel 打开中文列名不会变乱码；超过 20000 笔会截断并在文件末尾写明，绝不把残缺的文件装成完整的。两点安全边界：**不导卡密内容**（那是 `cards:manage` 的活，导订单的人拿不到钥匙，只拿到「这单发了 1 张」），并且买家自己填的联系方式、商品名一类以 `=` `+` `-` `@` 开头的值会先加一层保护再写入 —— 店家把导出的文件用 Excel 打开时，那应该是数据，不是可执行的公式
- 📊 **Admin 后台** — 概览看板（趋势图、订单结构、热销商品、买家排行、库存预警、优惠码成效、卡密健康、买家活跃）、商品/卡密/订单/用户/通知管理、操作审计日志、退款。后台自己的收件箱与前台同一套体验：侧栏「通知」上标未读条数（超过 99 写成 `99+`），通知页按类别分页签看（订单、促销、公告、系统、库存预警，本店真有的类别才出现），每条带中文类别名和「去处理 →」直达，一键「全部标为已读」回报实际改了几条
- 🔍 **审计日志筛选台** — 日志页可按四种条件组合查：操作类型（输入框联想本店真的出现过的动作名，输 `order` 就把 `order.refund`、`order.deliver` 都带出来）、关键词（在「操作 / 对象 / 详情」三列和「操作者用户名」里找包含关系，`%` 和 `_` 当普通字符处理，不会因为手滑输个星号就命中全部日志，输成员的名字就能翻出 TA 的全部动作）、操作者（填用户 ID，或按「只看系统操作」把对账、重试这类商店自己写下的记录单独捞出来）、日期区间（今天 / 近 7 天 / 近 30 天三个快捷档，也可以手填起止日期，结束那天算在内）。筛选结果同步写进地址栏，`/admin/logs?actor=3&since=2026-09-28` 这种链接可以直接贴进工单；用户详情页有「查看其操作记录 →」一键跳到 TA 名下；详情列被截断的行点一下就就地展开看全文。「操作者」列显示用户名而不是光秃秃的 ID（账号后来注销了也照样从历史里点名），认不出来时退回 `#用户ID`，商店自己写下的记录标「系统」
- 📄 **操作日志也能整批带走** — 日志页有「导出当前筛选 / 导出日志 CSV」，导出的就是屏幕上这一批：操作类型、关键词、操作者（含「只看系统操作」）、日期区间哪个在生效，下载的文件里就是哪个范围，一条不多一条不少（服务端在响应头里报实际行数，前端照实念出「已导出当前筛选的 312 条日志」，不让人自己数行）。列带日志 ID（就是列表第一列那个号，文件里的一行能指回屏幕上的一行）、时间（RFC3339，与页面上同一个时刻）、操作者（用户名，账号注销了退回 `#用户ID（账号已删除）`，商店自己写下的记「系统」）、操作类型、对象、详情、IP。文件开头带 UTF-8 BOM，Excel 打开中文列名不会变乱码；一次最多导 20000 条，超过就截断并在文件末尾写明「请缩小日期范围」，残缺的文件不会装成完整的。详情与对象里以 `=` `+` `-` `@` 开头的值会先加一层保护再写入 —— 店家把日志用 Excel 打开时，那应该是证据，不是可执行的公式
- 🧾 **订单列表也能贴链接** — 状态、关键词、指定买家、「需处理交付」与当前页码全部同步进地址栏，`/admin/orders?status=paid&user=7` 或 `/admin/orders?q=SEED-1&page=3` 打开就是那一批单子；地址栏里的页码超出实际页数时不会停在空白页上，而是退回最后一页（一笔都没有就回第 1 页）。「需处理交付」是按交付结果定的筛选，勾上它状态下拉会锁住，两种条件不叠加。列表底部的计数在这一页没行时只写「共 N 条」，不再出现「第 0–20 条 · 共 0 条」这种自相矛盾的话
- 🧭 **店招与页脚** — 后台直接编辑商店名称、Logo、公告、页脚文案与最多 8 条页脚链接，保存后前台立即生效
- 🎨 **商店主题** — 「外观」里的一个主题色会同时染色前台、后台与登录页（按钮、链接、徽标、聚焦圈与光晕按深/浅色各算一档），默认语言写进前台页面的 `lang`；改色不需要重新构建镜像
- 🔖 **浏览器标签页** — 商店名称就是标签页标题（商品页写成「商品名 · 店名」，订单页写成「订单号 · 店名」，后台写成「页面 · 店名 管理后台」），「网站描述」写进页面的 `description`，后台侧栏与登录页的门牌也用商店名称与 Logo
- 🖼️ **站点图标** — 「Logo 地址」就是浏览器标签页图标（前台与后台各有一份内置图标兜底），留空即回落到内置标识；只有 http(s)、本站相对路径或 `data:image/…` 能作为图标来源，其它协议的值会被服务端直接丢弃
- 🧭 **地址走到哪都不空白** — 前台与后台各自备有「页面不存在」一页：写下店里没有的地址，前台仍带顶栏与页脚，给出回店面/订单/个人中心的入口，后台在侧栏里列出当前角色真正能进的页面，标签页标题写成「页面不存在 · 店名」。不存在的商品也不再回显驱动层的 `record not found`：接口给中文文案加 `code: not_found`，页面显示「未找到该商品，它可能已经下架」并给出「去挑选商品」，加载失败时换成「再试一次」
- 🈶 **后台报错说中文** — 商品、分类、卡密、优惠码、系统设置、角色权限、审计日志、用户管理这八张页面的每一次拒绝都同时给机器码和中文文案：slug 重复是 `409 slug_taken`「这个 slug 已经有商品在用，换一个再保存」，改权限打错一个字是 `400 unknown_permission`「这条权限不在商店的清单里」，翻页翻坏了是 `400 invalid_query`「页码要是 1 以上的整数」，而不是 Casbin 的 `authz: unknown role "mystery"`、GORM 的 `constraint failed: UNIQUE constraint failed: products.slug` 或 `page must be a positive integer` 贴上屏。超级管理员改自己的角色/停用自己是「不能修改自己的角色，请让另一位超级管理员来调整」，扣积分扣穿了会说「要扣的积分比账号现有的还多，当前余额 N 分」。保存设置还多回一个 `restart_pending`：热重建失败时页面显示「设置已经保存，但这一项要等容器重启后才生效」，不再谎报「立即生效」。只有真的服务端故障才回「操作未能完成」，原始错误留在服务端日志的 `[catalog]` / `[system]` / `[identity]` / `[audit]` 行里
- 📣 **不跑 JS 也认得你的店** — 商店名称、「网站描述」、Logo 与默认语言由 Go 在返回 `index.html` 时就写进文档，并补齐 `og:title` / `og:description` / `og:image` 与 `twitter:card`：搜索引擎、论坛分享卡片和标签页首帧用的都是店家自己的文案，而不是构建产物里的默认值。相对路径的 Logo 会按当前访问的域名补全成绝对地址（`data:` 内联图标只作图标，不作为卡片图），管理后台的文档带 `robots: noindex`，且所有 HTML 一律 `no-cache`，改完刷新即生效
- 📷 **图片是上传的，不是抄路径的** — 后台的「封面图」和「Logo 地址」带上传按钮，选好文件就自动填回 `/uploads/…` 地址，不必登进服务器往数据卷里塞文件；买家个人中心也能直接上传头像（换图即把该买家上一张存在本店的头像删掉，不给数据卷攒孤儿文件）。服务端只认 png / jpg / gif（按文件头判断，改名的脚本会被拒；SVG 能带脚本，所以不收），单张不超过 2 MB 与 8192 px，文件名一律由服务端生成，三个入口各归各的主：商品封面是 `products:manage`，店面 Logo 是 `settings:manage`，买家头像只要登录
- 📐 **店家写多少字都不会撑破页面** — 商品名（最长 200 字）、公告里的长链接、发货说明、买家备注、站内通知正文、用户邮箱，这些都由店家或用户手打，商店按「先折行、再截断」排：不含空格的长串在词中间断开（`overflow-wrap: break-word`），而不是把商品卡、订单页、顶栏乃至整页推成横向滚动；商品详情页那条宽网格列允许收缩（`min-width: 0`），再长的名字也只在自己的列里换行，右边买家的价格框不受牵连。首页卡片的商品名最多两行、超出省略，旁边的分类徽标不会被挤出卡片。窄屏（不足 640px）顶栏收掉「进入后台」与「退出」，改由 ☰ 抽屉列出同一批入口，昵称按钮另有 8rem 的限宽。后台七个弹窗（新建优惠券、导入 / 生成卡密、转账与转账流水、人工发货、分类表单）比笔记本窗口还高时自己滚动（`max-height: calc(100vh - 32px)` + `overflow-y: auto`），不会再出现「最上面那个输入框在屏幕外、又没有任何地方可以滚」的死局
- 🛠️ **OpenResty 反代** — 适合用 OpenResty 跑其他服务、复用现有 vhost 的部署场景
- 🔒 **安全** — bcrypt 密码哈希、回调 HMAC 验签、Casbin RBAC、操作审计日志

## 💱 金额的口径：NL 与「商店积分」是两本账

商店里有两个长得像的数字，它们不是同一笔钱，页面也从不共用一个词：

| 这本账是什么 | 存在哪 | 显示成 | 谁在动它 |
|---|---|---|---|
| **商品价格 / 订单金额** | `products.price`、`orders.unit_price / discount_amount / total_amount`（都是整数） | `99 NL`、表单标签写「售价（NL）」 | 下单时原样（`strconv.Itoa`）发给 NodeLoc 从买家的论坛账户扣，查单读回来叫 `merchant_points` |
| **商店积分** | `users.points` + `point_entries.balance_after` | `积分`（签到奖励、后台加减分） | 只在店内流转，与 NodeLoc 没有任何兑换关系 |

因此这一版把「价格」相关的文案统一改成 NL，并且**页面上不会再出现 `¥`、「元」或「圆」**：前台店面价与详情页、下单确认、订单列表与详情、优惠码立减、后台概览的营业额与客单价、订单 CSV 的列名、退款到账的站内通知，写的都是同一个整数 NL。数据库里本来就是整数列，商品表单的步进也回落到 `1`——过去挂在输入框上的 `step="0.01"` 与「两位小数」的导出格式，会让人以为能卖 0.5 NL，而 NodeLoc 那一侧根本收不到这个数。后台给用户加减的「商店积分」仍然是「积分」，转账弹窗也会明说「这不是商店积分」，两个词不互换。

`provider_status`（查单/对账回报的 NodeLoc 侧状态）同样不再把英文原词甩给店家：`succeeded` / `pending` / `failed` / `paid` 等已知值有中文名，NodeLoc 若回报一个商店词表里没有的状态，页面写「未知道账状态（原词）」而不是干脆空白——原词保留在括号里，方便对着工单查，但它不再冒充一句人话。

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
| 权限范围 | 勾选 `openid`（必选，无法取消）、`profile`、`email`（需 NodeLoc 官方审核） |

> OAuth 四项（**接口地址 / Client ID / Client Secret / 回调地址**）里缺任意一项，商店都不会开始跳转登录，但**商店本身照常营业**：接口返回 503 并点名缺的是哪一项（用设置页上看得见的字段名，如「Client ID」「回调地址（请先在设置里填写站点域名）」），绝不把密钥值或服务商原文丢给浏览器。域名没填 → 回调地址生不出来，也是这句话。买家侧从浏览器发起的登录会带 `?oauth_error=not_configured` 回到登录页，绑定入口则回个人中心。
>
> **回调地址这一格可以留空。** 留空时商店按「站点域名」自己拼 `/api/v1/auth/oauth/callback`（协议跟随设置里的 `http`/`https`），手填的域名会被归一成纯主机名（`HTTPS://Shop.Example.COM/x/` → `shop.example.com`），手打了路径漏了协议头（`shop.example.com/api/v1/auth/oauth/callback`）也会自动补成完整地址；两者都不成立（填的是一句话而不是地址）时这一格会被清空，回到按域名自动生成。**把这一格清空并保存就是要求回到自动生成的地址**，商店不会把之前手填的旧地址偷偷续下来——曾经会，一条 `http://127.0.0.1:8080/login` 的旧覆盖能把回调钉死在本机，买家点登录后拿回一整页 HTML。设置页会替你盯住这类不自洽：回调的域名与站点域名不是同一个、路径不是 `/api/v1/auth/oauth/callback`、Client ID 填成了 `pay_` 开头的 Payment ID、凭据里带着空格或引号，都会以警告列出（只提醒，不拦保存），凭据齐不齐则由 `oauth_ready` / `oauth_missing` 说清楚，页面标题旁的「还登不了」「配置可疑」徽章用的就是它们。
>
> 买家那边到底卡在哪一步，后台 **设置 → 最近 NodeLoc 登录记录**（`GET /api/v1/admin/oauth-attempts`）按时间倒序摊开：发起授权 / 回调两步各自的成败、中文原因（`not_configured` / `rejected` / `unreachable` / `denied` / `expired` / `state` / `bind`）、当时报出去的回调地址，以及服务商原文。**授权码、`state`、Client Secret 和换回来的会话 token 一律不写进这张表**（入库前按正则洗一遍，长正文裁到 500 字），所以店家可以放心截图发给任何人排查。这张表只留最近 500 条——它记下的是「谁来过」，不是店家的永久访客名册。
>
> 登录失败不再是一句「服务器开小差了」：`oauth_not_configured`（503，本店参数没配齐）/`oauth_rejected`（502，NodeLoc 拒绝，如授权码已用过、Client ID/Secret 或回调白名单不匹配）/`oauth_unreachable`（502，连不上 NodeLoc）/`oauth_transaction_used`（授权事务已过期或已处理）分别给文案。服务端使用 `oauth_transactions` 保存 state hash、intent、回跳地址、过期时间和一次性消费状态；重复 callback 在 Token Exchange 前直接拒绝，不会再次兑换 authorization code。登录 callback 在服务端完成 Token/UserInfo 后才把本站 Session 交给前端，绑定流程也由服务端完成。服务商返回原文只进入脱敏后的 OAuth 诊断记录与服务端日志：会保留 `error`/`error_description`，但不会记录 code、state、secret 或 token。userinfo 认 `id`/`uid`/`user_id`/`sub`，用户名认 `username`/`preferred_username`/`nickname`/`display_name`，头像认 `avatar_url`/`picture`，也认 `{"data": …}` 外层包装和 HTTP 200 下的 `{"success": false,…}` 业务拒绝。

> **已按官方文档 `docs.nodeloc.com/api-reference/introduction` 对齐并修掉一个会让登录必败的 bug**：文档里 userinfo 的账号标识是**整数**（`{"id": 123, "username": "user1", "name": "user1", "avatar_url": …, "trust_level": 2}`），而商店原来把它按**字符串**解码——于是文档形态的论坛上，买家在 NodeLoc 那侧明明授权成功，换到令牌后却一定停在「资料里没有账号标识」。现在整数与字符串两种写法都能读（OIDC 的 `sub`、论坛的 `uid` 也照旧），**但一个账号标识都没有的资料会被明确拒绝**，不会拿空 key 去建号。

> **「绑定 NodeLoc」也重写了**：绑定是一次顶层浏览器跳转，带不上 SPA 的 `Authorization`；原来这条路由挂在鉴权中间件后面，点下去必然 401。现在分支是先换一个 10 分钟、`Type=bind` 的一次性令牌（`GET /api/v1/auth/oauth/bind-token`），把它放进跳转参数与 Cookie，回调时校验这个令牌与授权事务里的账号一致才写入绑定。会话令牌当绑定令牌用会被拒（`Parse` 只认 `access`），绑定成功后服务端直接 302 回个人中心并带 `oauth_bind=ok`，失败则带上可直接展示的中文原因。

商店侧的 Scope 在后台设置里填（默认 `openid profile`）：留空或漏写 `openid` 时商店会自动补上，因为 NodeLoc 要求授权请求必须带它；只有申请到 `email` 审核后才把它加进去，否则拿不到邮箱。

保存后记录 **Client ID** 和 **Client Secret**（只显示一次）。

#### 2.2 支付应用

访问 <https://www.nodeloc.com/payment/applications> → 创建应用：

| 字段 | 填什么 |
|---|---|
| 应用名称 | 你的商店名 |
| 网站地址 | `https://你的域名` |
| 回调地址 | `https://你的域名/api/v1/payment/callback` |

保存后记录 **三项凭据**（NodeLoc 只在创建时完整显示一次）：

| 凭据 | 样子 | 商店里填哪 | 用来做什么 |
|---|---|---|---|
| Payment ID | `pay_xxx` | 后台设置 · Payment ID | 只标识应用，出现在下单/查单的 URL 路径里，不参与签名 |
| Payment Token | `tk_xxx` | 后台设置 · Payment Token | 签 **下单 / 转账**：HMAC 密钥是 `hex(SHA256(tk_xxx))`，官方文档明确要求不能拿 token 原文当密钥 |
| Secret Key | 商户密钥 | 后台设置 · Secret Key | 签 **查单**、验 **回调**：按原文参与 HMAC，不做任何哈希 |

**Payment ID 必填，两把签名密钥填任意一格即可。** NodeLoc 的支付应用在不同版本里既发过 `tk_xxx` + 商户密钥两串，也只发过一串，而商店签名时用的是这串密钥的 `hex(SHA256(...))` 或原文——所以只有一串密钥的店直接填进 Secret Key（或 Token）就能收款。两格都空着时商店才拒绝下单，并点名「Payment Token 与 Secret Key（NodeLoc 的支付应用只给一串时，填进任意一个即可）」，不会拿空密钥去签名、也不会让买家付了款却对不上账。

**签名方式是一档一档试出来的，不是猜的。** 官方文档、NodeLoc 的控制台与本仓库上游那套确实收到过款的客户端，三者对「用什么密钥、带不带时间戳」的说法并不一致。商店因此按候选顺序依次尝试（`hex(SHA256(tk_xxx))` → `hex(SHA256(Secret Key))` → Secret Key 原文，各带/不带 10 位秒级时间戳），**同一路由最多四档**，一旦 NodeLoc 接受就记在本进程的「这一路由用这一档」表里，后续调用直接用，不再重复试。设置页的「测试支付网关」会把实测到的那一档写在结论后面（例如「本次实测：下单：Payment Token 的 SHA-256、带时间戳」），店家因此看得见商店究竟在用哪把密钥签名。

**只有「签名类」拒绝才会换下一档，业务拒绝一次就停。** NodeLoc 的应答分成两类，处理方式完全不同：说「签名无效 / 密钥不对 / 校验失败」这类签名话术，以及说不出原因的拒绝，才会推进到下一档；而「余额不足」「超出单笔限额」「转账功能未开启」「收款方不存在」「金额不合法」「重复下单」「Payment ID 不存在」「查单只认浏览器会话」「网关连不上」「时钟不同步」「参数没配齐」这些都是**业务判定**——NodeLoc 既然能回答这一笔业务，就说明它已经认了这把密钥，此时换密钥重试只会在扣钱的接口上把同一笔请求多发三次。因此一笔被业务理由拒绝的**转账就是 1 个 POST**，而这一档签名方式会被记下来（`remember`），下次直接用。签名话术优先于业务词：一句「签名无效，无法核对余额」仍然算签名问题。

**商店读的是编号，不是那句话。** NodeLoc 拒绝时按文档带一个数字：`{"status":400,"errors":[{"code":1001,"detail":"Invalid signature"}]}`。`detail` 跟着论坛的语言走，同一件事在英文界面写 "Insufficient balance"、在中文界面写「余额不足」，过去商店靠这句话判断要不要换密钥，于是同一个错误在不同 release 上行为不同。现在编号优先，句子里的词只作兜底（不带编号的 release）：

| 编号 | NodeLoc 的意思 | 商店的做法 |
|---|---|---|
| `1001` | 验签失败 | 换下一档签名方式（最多四档），全被拒才说「凭据不对」 |
| `1002` | 时间戳过期或无效 | 报 `provider_clock`：让店家同步宿主机时钟，不碰密钥 |
| `1003` | 调用方 IP 不在支付应用的白名单里 | 报 `provider_ip_blocked`：这一条在 NodeLoc 读密钥**之前**就跑完了，所以既不是「连不上」也不是「签名不对」，改凭据、换容器都没用，只能去支付应用加白名单；商店不会因此换密钥重跑 |
| `1004` / `1005` | 缺参数 / 参数无效 | 报请求本身的问题。转账遇到「缺金额参数」这一种会说清并**改用旧版字段名重试一次**（见下），其余参数问题不换密钥重跑 |
| `1006` | 这个 `order_id` 已经有过一笔支付 | 幂等回执：状态从 `data.status` 读（文档把状态放在这里，不在句子里），已付即入账、未付即把原本那笔支付地址回给买家 |
| `1007` / `1008` / `1009` | 订单 / 支付 / 应用不存在 | 按「这一单 NodeLoc 没见过」处理，不当成凭据坏了 |
| `1010` / `1011` / `1012` / `1013` | 余额不足 / 低于最小值 / 不能转给自己 / 触到每日上限 | **一次 POST 就停**并把 NodeLoc 原话送到店家面前；这些编号是在密钥被接受之后才可能出现的，所以同时把这一档签名记下来 |

**转账的金额字段叫 `points`。** 文档写的必填字段是 `to_user_id, to_username, points, order_id, timestamp, signature`，而本仓库上游那个客户端发的是 `amount`——NodeLoc 从来看不见它要的那个数，于是每一笔退款都被当成参数错误拒掉。商店现在首发 `points`（整数原样，文档允许最多两位小数），只有回 `1004`/`1005` 且明说缺的是金额时才补发一次 `amount` 形状，其他拒绝不重试。两个收款方字段都取自买家绑定的那个 NodeLoc 账号；一个都没绑的店，商店在发出请求**之前**就拒绝并提示先去让用户绑定，不会拿空收款方去碰运气。买家只有 uid、没有用户名时，空的 `to_username` 会同时从签名串和请求体里去掉——只删一边会让 NodeLoc 重算出不同的签名，把一笔正常退款拒成「被篡改」。

**回调与查单里的钱是小数字符串。** `amount` 可能是 `"100.00"`，`merchant_points` / `platform_fee` 可能是 `90.5`。商店按积分取整读回（`9.5` 积分是一笔真实结算，不会被当成「金额为 0」而拒收），下单送出去的仍是那个整数定价。

**NodeLoc 的原话会送到店家面前。** 拒绝应答被建模成带类型的错误（`domain.ProviderRefusal{Message, Signature}`），退款 / 转账失败时后台看到的是服务商自己写的那句话（「NodeLoc 原文：余额不足」），而不是被商店包装过的通用错误；买家侧只显示「暂时无法向 NodeLoc 确认支付结果，请稍后再试」这类不含内部信息的说法。查单对账的应答还带上「这一单走了哪条路径核实」（`provider_via`，商家可见、买家不可见），走下单核实时后台会显式标出「经由下单核实」并附原因。

**签名规则（出站与回调同一套）**：去掉 `signature`，其余参数（含值为空的参数）按 key 的 ASCII 升序拼成 `k1=v1&k2=v2…`，UTF-8 编码后做 HMAC-SHA256，输出小写十六进制。

**时间戳**：官方文档要求下单 / 查单 / 转账都带一个 10 位秒级 `timestamp`，与 NodeLoc 服务器相差超过 5 分钟即拒绝。商店按请求即时生成并纳入签名，因此宿主机时钟必须同步（NTP）；「测试支付网关」遇到这类拒绝会直说「这台服务器的系统时间与标准时间相差过大，请在宿主机上同步时钟，支付凭据本身没有问题」，不会把时钟问题误报成密钥问题。

> **查单（`POST /payment/query/{payment_id}`）在真实的 NodeLoc 上是论坛后台的浏览器接口，不是服务器对服务器接口。** 实测过各种请求形状——表单或 JSON、带 Payment Token、带 Bearer、带 CSRF 头、带 Origin/Referer——NodeLoc 一律回 `403` 纯文本 `["BAD CSRF"]`，因为这条路由属于商户自己的登录会话，商店的服务器调不动它。商店因此把「核实到账」建在两条服务端能真正调通的路径上：
>
> 1. **浏览器回调**：付款完成后的 302 里带签名，验签即入账；
> 2. **下单回执**：把同一个 `order_id` 再发一次 `POST /payment/pay/{payment_id}/process`，NodeLoc 不会重复扣钱，而是回「Order already exists with status …」——状态词就是这一单在 NodeLoc 侧的到账状态。前台「我已支付，去确认」、后台「查单对账」与每 3 分钟一轮的自动核实走的都是这条路径。
>
> 「测试支付网关」**先证明下单，再报查单**。真实 NodeLoc 的查单永远调不动，所以只看查单的自检会在凭据全错时照样亮绿灯——店家测过「一切正常」，买家那边却个个「无法支付」。这一版把顺序倒过来：按钮先在本店已付款、已完成的订单里找到一笔 NodeLoc 给过交易号的（最多翻最近 20 单），把它的 `order_id` 原样再发一次 `POST /payment/pay/{payment_id}/process`。NodeLoc 不会重复扣钱，回 1006「Order already exists」即证明**这把签名密钥它认**；此时查单再回 `["BAD CSRF"]` 才被说成「支付网关可达，但 NodeLoc 的查单接口只接受论坛后台的浏览器会话，商店改用下单回执与支付回调核实到账，收款与发货不受影响」（`provider_guarded`，判定为**可达**）。若这一笔重发被 NodeLoc 当场拒了，结论就直接是那个拒绝原因（`provider_rejected` / `provider_clock` / `provider_ip_blocked`），不再拿「网关可达」安慰店家。**一家还没收过钱的店没有可重发的单**，此时按钮老实回答 `not_verified`：「还没有测出下单凭据是否可用……请完成并支付一笔真实订单再按」——这一条 `ok:false`，因为签名确实没被证明过。整次自检在最坏情况也**不超过四个 POST**（四档签名方式试完为止），不会把一笔真订单反复推到收银台。若该路由回的是 404——无论 NodeLoc 文档那种 `{"status":404,"error":"Not Found"}` 正文，还是实测到的**空正文 404**——说明这个地址上根本没有后台填的那个支付应用：商店报 `payment_id_unknown` 并点名设置页里对应的那一格（空正文 404 点名「Payment ID」，带论坛 404 网页的点名「支付 API 地址」），因为此时问题在那一格，而不在密钥。

> **查单被闸门拦下是「这一段时间」的结论，不是永久判决。** 网关只会把路由标记为「有会话闸门」**15 分钟**，窗口内不再重复敲同一堵墙、直接走下单核实；窗口一过会自动再试一次查单——NodeLoc 若哪天把它放开成服务器接口，商店无需改代码就能用回这条路。与之相对，**一次 HTTP 200 的网页（例如网关挂了吐出的 HTML）不会被当成会话闸门**：那属于「这一句应答读不懂」，本次调用改走下单核实，但路由不会被永久摘掉，下一次查单照旧尝试。

**实测的 NodeLoc 路由形状**（2026-10-01 对 `www.nodeloc.com` 的只读探测，不带任何凭据，用不存在的 `pay_` 编号）：

| 请求 | NodeLoc 实际回什么 | 商店据此说什么 |
|---|---|---|
| `POST /payment/pay/{未知编号}/process` | `404`：实测见过 `Content-Type: text/html` + **正文 0 字节**，也见过文档写的 `{"status":404,"error":"Not Found"}` | 两种都读成「这个 Payment ID 在 NodeLoc 上没有对应的支付应用，请核对设置页的『Payment ID』」——一次请求就停，绝不换密钥重跑 |
| `POST /payment/transfer/{未知编号}` | 同上 | 同上 |
| `POST /payment/query/{任意编号}` | `403` `text/plain` `["BAD CSRF"]`（会话闸门在查应用之前就跑完了，所以未知编号也看不到 404） | 「查单只认论坛后台的浏览器会话」，改走下单核实，15 分钟后再试 |
| 这条路径在这个域上根本不存在（例如支付接口没装在登录镜像域上） | `404` + 论坛自己的「找不到页面」HTML 页 | 「这个域上没有支付接口的路由，请核对设置页的『支付 API 地址』」 |
| `GET /oauth-provider/authorize`（参数齐全但未登录） | `302` 到 `/login`，**即使 Client ID 从来没存在过也照样跳登录** | 正常：授权页本来就要登录。也正因为它对真假 Client ID 都给这个跳转，**能拼出授权链接不代表凭据可用**——所以「测试 NodeLoc 登录」不看这里，改看 token 端点 |
| `POST /oauth-provider/token`（凭据不对） | `400` `application/json` `{"error":"invalid_client","error_description":"Invalid client credentials"}` | 商店把标准 OAuth 错误码翻成中文（`invalid_client` → 「Client ID 与 Client Secret 对不上这个 OAuth 应用」，`invalid_grant` → 「授权码已经用过或过期，让买家重新点一次」），并把 NodeLoc 原话附在后面 |
| `POST /oauth-provider/token`（凭据对，但码不存在） | `400` `{"error":"invalid_grant", …}` | 这就是「测试 NodeLoc 登录」发的那个假码：客户端被认出、只有码被拒 → 判定**凭据可用**（`ok:true`） |
| 「测试 NodeLoc 登录」把「NodeLoc 站点地址」填成了商店自己的域名 | 商店的 SPA 对未知路径回 **HTTP 200 + 一整页 HTML** | 光看 2xx 会是又一个假绿灯；商店读**正文形状**：带 `<!doctype html>`/`<html>` 的不是 OAuth 应答 → `route_missing`，点名「请把『NodeLoc 站点地址』填成论坛本体」 |
| `GET /oauth-provider/userinfo`（无 token） | `401` `application/json` `{"error":"invalid_token"}` | 同上，翻成中文并保留原话 |
| `User-Agent` 为 Go 默认值 | 与浏览器 UA 收到的应答**完全一致**（支付与 OAuth 路由都没被 Cloudflare 拦） | 商店不需要伪装浏览器身份，也不必为此加请求头 |

**同一个 404 的正文并不固定**：实测 `www.nodeloc.com` 对未知编号回过 `Content-Type: text/html` 的**0 字节** 404，也回过文档那种 `{"status":404,"error":"Not Found"}`；而一条论坛路由器根本不认识的路径回的是 `{"errors":["找不到请求的 URL 或资源。"],"error_type":"not_found"}`，浏览器形状的请求还会拿到整页 46 KB 的「找不到页面」HTML。请求带不带 `Accept: application/json`、编号是否存在，都会改变正文形状——所以商店**不靠状态码猜哪一格错了，而是读正文形状**：带 `errors` 数组 / `error_type`，或是 HTML 网页 → 这是论坛路由器的话，说明这个域上根本没有支付接口的路由，点名「支付 API 地址」；空正文和 `{"status":404,"error":"Not Found"}` → 这是支付应用自己的话，说明路由在、只是没有这个编号，点名「Payment ID」。两种都只发一次请求，都不推进签名档，也都不留「这条路不通」的记忆。

两个 404 因此是两件事：一个说「编号抄错了」，一个说「地址发错域了」，都不关密钥的事。商店过去只把带 JSON 正文的 404 认成「Payment ID 不存在」，空正文的那个被读成「签名被拒」——于是同一笔无效编号在收钱的接口上连发四个 POST，结论还写着「请核对 Payment Token」。**店家由此看见的症状是「nodeloc 全都用不了、改密钥也没用」**，所以现在这两种形状各自点名正确的那一格，且都只发一次请求；把 Payment ID 改对，下单立刻恢复（404 不会留下「这条路不通」的记忆）。

> 后台的「退款」也真的把钱退回去：商店调用 `POST /payment/transfer/{payment_id}`（Payment Token 签名，收款方取买家绑定的 NodeLoc 账号），NodeLoc 受理之后才把订单标成已退款。买家是邮箱注册、没绑 NodeLoc 时，商店会直接拒绝并提示人工处理，不会假装已经退款。**一笔转账被 NodeLoc 拒绝就是一次请求**：余额、限额、转账开关、收款方这类业务拒绝不会换密钥重跑（详见上文签名一节），店家在后台看到的是 NodeLoc 的原话。

> **重复下单不会让买家再付一次**。同一个 `order_id` 第二次请求下单时，NodeLoc 回的是「Order already exists with status …」而不是新的付款页。商店把这句识别成类型化的拒绝：状态已是 `paid`/`completed` → 立刻查单、当场入账并发卡（钱已经扣了，这一单就此结掉）；状态仍未付 → 把该单原本那笔支付的付款地址原样回给买家，继续付同一单。前台看到这笔支付已经到账时不会再把人送去收银台，而是直接跳到该订单页。

#### 2.3 回调接口一览（与代码路由一致）

| 用途 | 方法 | 接口路径 | 谁来调用 | 之后发生什么 |
|---|---|---|---|---|
| OAuth2 登录回调 | `GET` | `/api/v1/auth/oauth/callback` | NodeLoc OAuth 授权后浏览器跳转 | 校验 `state` + 换 token → 302 回商店前端 `/oauth/callback`（token 放 URL fragment，不落日志）；AJAX 请求则直接返回 JSON |
| 支付结果回调 | `GET` | `/api/v1/payment/callback` | NodeLoc 支付完成后浏览器跳转 | HMAC-SHA256 验签 → 幂等履约发卡 → 302 到 `/orders/{订单号}` |
| 支付结果回调 | `POST` | `/api/v1/payment/callback` | 兼容路由，NodeLoc 本身不会调 | 同上验签与履约，返回 `{"success":true}` |
| 买家查单 | `POST` | `/api/v1/payment/orders/{订单号}/reconcile` | 商店前台「我已支付，去确认」 | 用商户密钥主动向 NodeLoc 查询该单 → 已付则当场入账发卡，返回结构化结果（`settled` / `provider_status` / `retryable` / `checked_at`）；买家的应答里**不含**签名档、路由原因等服务商内部信息 |
| 商家查单 | `POST` | `/api/v1/admin/orders/{订单号}/reconcile` | 后台订单详情的「查单对账」 | 同上，管理员可对任意待支付订单执行。额外回报 `provider_via`（这一单由哪条路径核实：`query` 还是 `reprocess`）与 `provider_note`（改走下单核实的原因，即 NodeLoc 那句原话），页面据此标出「经由下单核实」 |
| 批量查单 | `POST` | `/api/v1/admin/reconcile/pending` | 后台订单列表的「批量查单对账」 | 一次问完所有「已拿到 NodeLoc 交易号却仍显示待支付」的订单（每批 20 笔），返回逐笔原因；本批若普遍走了下单核实，列表顶部会提示「本轮改由下单回执确认」，每行也带「经由下单核实」标记 |
| 待交付队列 | `GET` | `/api/v1/admin/orders?attention=undelivered` | 后台「需处理交付」筛选 | 列出已收款但还没发货/待人工/待补货的订单；该参数会取代 `status` 筛选 |

> NodeLoc Payments 的 HMAC 接口**只有浏览器跳转式回调**，付款完成后没有任何服务端推送（IPN）通知商店。因此到账不能只押在那一次 302 上：买家关页面、签名对不上、回调丢失，都会由「主动查单」这条路径补回来——前台的确认按钮、订单页的自动轮询，以及后台的查单对账，走的都是 NodeLoc 的查询接口。

> 回调验签失败时商店不会直接判定「没付款」：先用商户密钥按「含空参」和「去掉空参」两种拼法各验一次，仍不匹配就自动调用 NodeLoc 的查询接口核实这一单，NodeLoc 记为已付就当场入账并发卡。也就是说，**买家已付款但回调签名对不上时不需要再付一次**；后台设置里填错 Secret Key 时，这条路径同样会把订单核对出来。

> 文档在两处分别把回调密钥写成 Secret Key 原文和 `sha256(Payment Token)`，商店**两种密钥都验**，任一命中即认账；跳转里缺 `transaction_id` 也不影响入账，商店会拿该单原本存下的交易号补上（查不到交易号时也只是记空，不会把已验签的付款拒之门外）。这条兜底之外，商店自己每 3 分钟还会向 NodeLoc 核实一次超过 10 分钟仍显示待支付、且已有交易号的订单（每轮 10 笔），所以浏览器被直接关掉的订单也会自己结掉。

> 两个地址都要**逐字**填进 NodeLoc 控制台（含 `/api/v1` 前缀），并与你商店初始化时填的域名完全一致；初始化向导和后台设置页的回调地址留空即自动生成，输入框占位符显示的就是完整地址，照抄到 NodeLoc 即可。
> 前端还有 `/oauth/callback`、`/orders` 等路由属于商店自己的页面，**不要**填到 NodeLoc 的回调地址里。

#### 2.4 支付结果确认与错误码

所有支付类接口出错时返回同一个结构，前端只按 `code` 分支，不猜 HTTP 状态：

```json
{ "error": true, "code": "unsettled", "message": "NodeLoc 还没有这单的到账记录…", "retryable": true, "detail": "…" }
```

`detail` 是 NodeLoc 自己的措辞，可能包含凭据与内部状态，**只在 `/api/v1/admin/...` 路径上返回**；商店前台永远收不到这个字段。`retryable` 为真表示稍后再查一次就可能变好（联系不上 NodeLoc、尚未到账），为假则要点别的按钮或找店家（金额不符、交易号归属别的订单、没配好支付）。

| `code` | HTTP | 含义 | 买家该怎么做 |
|---|---|---|---|
| `invalid_input` | 400 | 参数不合法 | 检查后重新提交 |
| `forbidden` | 403 | 订单不属于当前账号 | 换登录身份 |
| `amount_mismatch` | 400 | NodeLoc 记录的金额与本单不一致 | 停止自动入账，联系店家核对 |
| `foreign_transaction` | 409 | NodeLoc 把这笔交易归属到其他订单 | 联系店家核实 |
| `callback_invalid` | 422 | 回调未通过验签（商店已自动查单核实） | 等页面刷新或再点确认 |
| `no_transaction` | 409 | 商店还没拿到这单的交易号 | 点「继续支付」重新发起 |
| `unsettled` | 409 | NodeLoc 暂无到账记录（可重试） | 已扣款请稍候再查 |
| `not_complete` | 409 | NodeLoc 回报支付未完成（可重试） | 继续付款或稍后再查 |
| `refund_recipient_unknown` | 409 | 买家未绑定 NodeLoc 账号，无法原路退款 | 转人工处理 |
| `not_configured` | 503 | 支付凭据没填齐（Payment ID 为空，或两把签名密钥都是空的） | 店家去后台设置 |
| `payment_id_unknown` | 503 | NodeLoc 在这个地址上找不到后台填的 Payment ID（实测是**空正文的 404**；文档那种 `{"status":404}` 正文也算）。若 404 带的是论坛自己的「找不到页面」网页，则是「支付 API 地址」指错了域 | 店家核对 Payment ID 与支付 API 地址 |
| `provider_guarded` | 502 | NodeLoc 的查单只认论坛后台的浏览器会话，商店改用下单回执核实（可重试；这一判定只维持 15 分钟，之后会自动重敲查单） | 稍后再查一次 |
| `provider_clock` | 503 | NodeLoc 拒绝商店的请求时间戳，宿主机时钟没同步（与密钥无关） | 店家同步 NTP |
| `provider_unreachable` | 502 | 联系不上 NodeLoc（可重试）；网关回的是网页而不是接口应答时也归在这里，不会因此把查单路由永久判死 | 稍后再查一次 |
| `provider_rejected` | 502 | NodeLoc 拒绝签名请求，多为凭据不匹配（商店已把试过的签名档一并写进日志；余额、限额、开关、收款方这类**业务**拒绝不算在这里，只发一次请求并把 NodeLoc 原话交给店家） | 店家核对 Payment Token |
| `not_found` | 404 | 订单、商品、分类等记录不存在（文案已是中文，不再回显驱动层的 `record not found`） | 回列表页 |
| `product_unavailable` | 404 | 商品已下架 | 换商品 |
| `insufficient_stock` | 409 | 卡密库存不足 | 等店家补货，补货后自动交付 |
| `not_payable` | 409 | 本单当前不可支付或已交付 | 刷新订单 |
| `internal` | 500 | 未预期的后端错误 | 重试或联系店家 |

**两条后台自愈循环**（`maintenanceLoop`，启动 20 秒后跑第一轮，之后每 3 分钟一次）：

1. **自动查单**：挑出「已拿到 NodeLoc 交易号、却仍显示待支付、且已放置超过 10 分钟」的订单（每轮 10 笔）向 NodeLoc 查询，已付则当场入账。买家关掉付款页不再回来，钱也不会卡在待支付。10 分钟的门槛是为了不抢正在轮询的付款页。
2. **交付重试**：挑出 `paid` / `completed` 但履约状态仍是 `pending` / `waiting_stock`、且付款已超过 1 分钟的订单（每轮 50 笔）重新发货。已绑给本单的卡密会被直接复用，不会重复占库存。**补货不再需要等这一轮**：导入、生成或重新启用卡密成功后，商店立刻按商品把它的等待队列放出来（`POST /admin/products/:id/cards`、`.../cards/batch-add`、`.../cards/generate`、`.../cards/batch-status` 的响应里 `released` 就是当场交付的笔数），清扫只是万一没放成时的兜底。

因此后台的「需处理交付」队列（`?attention=undelivered`）只应包含自动重试搞不定、需要人来做的部分：人工发货、以及 NodeLoc 尚未确认到账的单子。

#### 2.5 优惠码的拒绝原因

试算与下单走的是同一套规则，因此两边返回的 `code` 完全一致——买家不会在输入框里看到「还能用」，按下付款却被换成另一句理由。响应结构与支付类相同（`error` / `code` / `message`），HTTP 状态一律 `422`：

| `code` | 含义 | 买家看到的话 |
|---|---|---|
| `coupon_not_found` | 没有这个码 | 没有这个优惠码，检查有没有打错。 |
| `coupon_inactive` | 店家把码关掉了 | 这个优惠码已被店家停用。 |
| `coupon_not_started` | 还没到生效时间 | 这个优惠码还没到生效时间。 |
| `coupon_expired` | 过了失效时间 | 这个优惠码已经过期了。 |
| `coupon_exhausted` | 总量额度已被占满 | 这个优惠码的额度已经用完了。 |
| `coupon_used` | 该账号已用过（含尚未支付的单） | 你的账号已经用过这个优惠码。 |
| `coupon_min_amount` | 订单金额未达门槛 | 订单金额还没达到这个优惠码的使用条件。 |
| `coupon_not_applicable` | 范围不含本商品 | 这个优惠码不能用在当前商品上。 |
| `coupon_disabled` | 商店关掉了优惠码功能 | 商店暂未开启优惠码。 |
| `coupon_invalid` | 其他内部错误（不该出现） | 优惠码无法使用。 |

限用的口径值得说清楚，因为它是店家真金白银的预算：

- **每人限用**统计该账号名下仍在的单（待支付也算），已取消的不算。买家下了单又没付款，额度已经占住，取消后自动释放。
- **总量限用**同样按仍在的单计算，而不是等 NodeLoc 确认收款才扣。否则一张「全网限 1 份」的码在付款确认之前会被卖给每一个来问的人。
- **100% 折扣不会把订单压成 0 NL**：NodeLoc 收不到 0 数额的付款，所以最多减到只剩 **1 NL**，订单照常走支付与交付。

#### 2.6 商品 / 分类 / 卡密 / 优惠码接口的错误码

后台那几张表单（商品、分类、卡密、优惠码）出错时同样返回 `{ "error": "中文文案", "code": "机器码" }`，页面直接把 `error` 显示给店家，不再把 Go 驱动层的英文原文（`record not found`、`constraint failed: UNIQUE constraint failed: products.slug`）当成店家的话贴上屏。数据库撞了唯一索引会被认出来换成对应规则，`detail` 字段只在 JSON 请求体真的无法解析时出现，用于自查表单，不代表会展示。

| `code` | HTTP | 含义 | 店家该怎么做 |
|---|---|---|---|
| `invalid_request` | 400 | 请求体不是合法 JSON | 刷新后台重试，或按 `detail` 检查提交内容 |
| `invalid_id` | 400 | 地址里的编号不是数字 | 从列表页重新进入 |
| `invalid_input` | 400 | 表单某一项不合规则，文案已指出是哪一项（名称/slug 必填、价格不能为负、优惠码范围、每人限用不能为负……） | 按提示补填后重新保存 |
| `invalid_query` | 400 | 查询或批量操作条件不对（排序方式不认识、一次生成数量超出 1~500、前缀不合规、没有勾选卡密） | 调整筛选或勾选后再操作 |
| `invalid_product_type` | 400 | 商品类型只认 `card` / `manual` | 重新选择类型 |
| `invalid_card_status` | 400 | 卡密状态不是可写入的取值 | 用「可用 / 停用」两个按钮切换 |
| `manual_product` | 400 | 手动发货商品没有卡密库存 | 换成卡密类型，或改走人工交付 |
| `slug_taken` | 409 | 商品或分类的 slug 已被占用 | 换一个 slug 再保存 |
| `code_taken` | 409 | 优惠码已存在 | 换码，或去编辑已有的那张 |
| `coupon_*` | 422 | 优惠码不可用，见 2.5 | 按 2.5 的话处理 |
| `not_found` | 404 | 商品、分类、卡密等记录不存在或已删除 | 回列表页 |
| `internal_error` | 500 | 真正的服务端故障，客户端只看到这一句；原始错误写进服务端日志（`[catalog]`） | 查容器日志后联系开发者 |

#### 2.7 系统设置 / 角色权限 / 审计日志 / 用户接口的错误码

后台剩下的四张页面（系统设置、角色权限、审计日志、用户管理）走同一套约定：`{ "error": "中文文案", "code": "机器码" }`，Casbin 与 GORM 的英文原文只留在服务端日志（`[system]`、`[identity]`、`[audit]`）里。

| `code` | HTTP | 含义 | 店家该怎么做 |
|---|---|---|---|
| `invalid_request` | 400 | 提交的内容无法解析，或缺了必填字段（比如角色权限整个字段没带上） | 刷新后台重试 |
| `invalid_input` | 400 | 设置项或用户操作不合规，文案直接说是哪一项（Client ID/Secret 不能为空、不能修改自己的角色、不能停用自己的账号、要扣的积分比账号现有的还多……） | 按提示改，涉及自身账号的操作换另一个超级管理员来做 |
| `invalid_query` | 400 | 审计日志的查询参数不合式（列表与 CSV 导出用的是同一套解析）：`page` / `limit` 不是 1 以上的整数，`actor` 既不是用户 ID 也不是 `system`，`since` / `until` 不是 `2026-09-29` 这样的日期，或开始日期晚于结束日期 | 用日志页的筛选控件与日期快捷档，别手改地址栏 |
| `already_installed` | 409 | 商店已经初始化过，重复提交安装向导 | 直接去「系统设置」改 |
| `not_installed` | 503 | 商店还没跑完初始化 | 先走完安装向导 |
| `authz_unavailable` | 503 | 权限服务还没就绪 | 重启商店容器 |
| `super_admin_locked` | 400 | 超级管理员的权限由系统保留，不在角色编辑页里改 | 用管理员/运营/客服承载细分权限 |
| `unknown_role` | 400 | 商店里没有这个角色 | 从角色列表里选 |
| `malformed_permission` | 400 | 权限写成 `"products"` 这样缺了操作 | 用「资源:操作」两段式，或直接勾选 |
| `unknown_permission` | 400 | 权限不在服务端目录里（路由根本不检查它，存下来等于没授权） | 只从 `/api/v1/admin/permissions` 返回的清单里取 |
| `unauthenticated` | 401 | 请求根本没带登录凭证（会话已过期的页面、直接抄来的链接） | 重新登录 |
| `invalid_token` | 401 | 凭证读不出来或已失效，前台与后台都会自动跳回登录页 | 重新登录 |
| `account_disabled` | 403 | 账号已被停用 | 联系管理员 |
| `not_found` | 404 | 页面还在调一个当前版本没有的接口 | 强刷浏览器缓存（HTML 一律 `no-cache`，刷新即拿到新版） |
| `empty_upload` | 400 | 表单里没带上图片 | 重新选文件 |
| `invalid_image` | 400 | 文件头不是 png / jpg / gif（改名伪装或 SVG 都会被拒），或像素尺寸超过 8192 | 换一张真正的图片 |
| `image_too_large` | 413 | 图片超过 2 MB | 压缩后再传 |
| `internal_error` | 500 | 真正的服务端故障 | 查容器日志后联系开发者 |

保存系统设置多回一个 `restart_pending`：`false` 表示运行时已重建、改动立即生效；`true` 表示配置已经写进数据库，但热重建失败（原始原因在服务端日志里），页面会显示「要等容器重启后才生效」，不再谎报「立即生效」。后台两张连接测试卡（OAuth / Payment）也同口径：配置不完整时说的是「NodeLoc 登录还没配置完整：请填好…」，不会把 `nodeloc oauth configuration is incomplete` 贴上屏；只有服务商自己的返回原文仍然照实附上，方便对照 2.2 的签名规则。

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

> 商店无需在 nginx 里挂静态资源：页面、CSS/JS、图片都由容器内的服务直接吐出。数据卷 `./data:/app/data` 保存 SQLite 库与初始化配置，`./uploads:/app/uploads` 存放图片：后台的「封面图」和「Logo 地址」都带上传按钮，选好文件就自动填好 `/uploads/...` 地址，不必登进服务器。仍然想自己放文件也可以——写进宿主机的 `./uploads/`，在表单里手填地址即可。

### Step 5 · 应用内初始化向导

浏览器访问 `http://IP:8080`（配好域名后访问 `https://你的域名`），未初始化时会自动跳到管理后台的初始化向导 `/admin/setup`，三步完成：

1. **站点与数据库** — 商店名称、访问域名、数据库驱动（默认 SQLite；选 MySQL 时填 Step 1 的连接信息）
2. **NodeLoc 集成** — OAuth 的 Client ID / Secret（必填）+ 支付的 Payment ID / Payment Token / Secret Key（三项可稍后在设置中补填，缺任意一项商店不会开始收款）
3. **管理员账号** — 创建首个账号，它拿到的是**超级管理员**（唯一能授予「管理员」及调整后台账号角色的角色），保存后直接进入后台登录

> 提交时如果数据库连不通，会显示错误提示让你重填，**不会破坏配置**。
> **保存即生效**，不用重启容器；无需编辑任何 yml / ini 配置文件。
> 之后随时可在后台 **设置** 页修改：密钥以 `********` 掩码显示，**保持掩码即不修改**，把输入框清空保存则真的删除该凭据（清空 Client Secret 会被校验拦下，不会让你锁死登录）。OAuth 与支付网关各有连通性测试按钮。**「测试 NodeLoc 登录」不再只拼一条授权链接**——实测论坛对 `/oauth-provider/authorize` 的跳转，无论 Client ID 存不存在都会 302 到它自己的登录页，所以能生成链接根本不证明凭据能用；它改成向 `/oauth-provider/token` 换一次一个**不可能存在的授权码**（只发一个 POST，不碰任何账号、不消耗买家登录），按 OAuth 自己的错误码判定：认这对凭据（`invalid_grant`：客户端已知、只是这个假码被拒）/ `invalid_client`（Client ID 与 Secret 对不上）/ `unsupported_grant_type`、`invalid_scope`、回调不匹配、连不上、以及**这个地址根本不是论坛本体**（商店自己的 SPA 会用 HTTP 200 回一整页 HTML 给未知路径，光看 2xx 就是又一个假绿灯，这里认成「这个地址上没有 NodeLoc 的 OAuth 接口」）。支付测试就是按文档对 NodeLoc 的查单接口签一次名。


### Step 6 · 验证支付

1. 用 Admin 账号登录后台 → **商品** → 新建一个商品 + 导入几个测试卡密
2. 退出登录，用另一个 NodeLoc 账号或邮箱注册普通用户
3. 下单购买 → 跳转到 NodeLoc 支付页 → 用 NodeLoc 积分（NL）付款
4. 支付完成后浏览器会跳转回你的 `/api/v1/payment/callback`，自动发货，卡密会显示在订单详情

## 🔌 接口一览

所有接口都挂在 `/api/v1` 下。标注「公开」的不需要登录，其余需要 `Authorization: Bearer <access_token>`；后台接口在此之上还要该项资源的 Casbin 授权，被拒时 `403` 的响应体会带 `permission` 与 `your_role`，后台页面正是据此把按钮藏起来的。

| 分组 | 路由 | 需要的授权 |
|---|---|---|
| 商店状态 | `GET /system/status`（公开） | — 返回安装状态、商店名/标语/简介/Logo/公告/页脚文案与链接、注册/签到/优惠码三个开关，以及 `theme`（`primary` 主题色、`locale` 前台语言）；两端据此上色，并把商店名与简介写成标签页标题和 `description` |
| | `POST /system/install`（公开，仅未安装时可用） | — |
| 会话 | `POST /auth/register`、`POST /auth/login`、`POST /auth/logout`、`POST /auth/refresh` | 登录（refresh 用 refresh_token 换新的 access/refresh 对）。`/auth/register` 受设置里的「允许注册」控制：关掉时回 403 `registration_disabled`（中文提示改用 NodeLoc 登录），前台的「注册」入口与注册页也一并换成「本店已关闭注册」；这个开关在旧的安装文档里可能压根不存在，缺省一律按「开」处理，升级不会把一家没关过注册的店悄悄锁死 |
| | `GET /auth/oauth/initiate`、`GET /auth/oauth/callback`（公开） | — 见 2.3 |
| 买家资料 | `GET /auth/me`、`PATCH /auth/me`（昵称、简介、联系邮箱、头像） | 登录 |
| | `POST /auth/me/avatar`（表单字段 `image`） | 登录 只凭会话，不需要任何后台授权。图片写进 `uploads/avatars`、文件名由服务端生成，账号随即指向新图，**上一张本店自己存的头像同时删除**；限制与后台上传一致（png / jpg / gif、2 MB、边长 8192 px） |
| | `GET /auth/me/permissions` | 登录 返回 `is_staff`、当前角色与逐项授权，前端权限的唯一来源 |
| | `GET /auth/me/points` | 登录 积分流水，分页 |
| | `GET /auth/checkin/status`、`GET /auth/checkin/history`、`POST /auth/checkin` | 登录 签到状态、日历与当日签到 |
| | `POST /auth/bind-oauth`、`DELETE /auth/unbind-oauth`、`POST /auth/me/sync-oauth` | 登录 绑定/解绑 NodeLoc，并把手上的授权换成正经身份 |
| 商店目录 | `GET /store/products`、`GET /store/products/:slug`、`GET /store/categories`、`GET /store/stats`（公开） | — 列表支持 `q`、`sort`（留空按店家排序，另可选 `sales` / `price_asc` / `price_desc` / `newest`，写错直接 `400`）、`category`、`featured`、`in_stock`、`limit`/`offset` |
| | `GET /store/coupons`（公开） | — 前台促销架：只列店家勾了「在前台展示」、当前真的能用得上的码，返回 `{ "data": [...], "enabled": true/false }`。判据与下单试算完全一致（同一套生效窗口比较，限量额度按非取消订单的真实占用计算，含未付款），所以架子上不会出现试算时被拒的码；还没到生效时间、已过期、店家停用的都不露面，`enabled_coupons` 关掉时直接给空数组。每条带 `code`、`discount_type`、`discount_value`、`min_order_amount`、`scope`、`product_id`、`category_id`、`per_user_limit`、`description`、`valid_until`，以及 `remaining`（不限次时不出现）；按失效时间近的排前面。谁都能读，不需要登录，也不消耗任何额度 |
| | `POST /store/coupons/quote` | 登录 试算优惠码，返回 `discount` / `payable` / `original_total`；失败见 2.5 |
| 下单支付 | `POST /payment/orders`、`POST /payment/create`、`GET /payment/orders`、`GET /payment/orders/:order_no`、`POST /payment/orders/:order_no/reconcile` | 登录 下单时可带 `coupon_code`，折扣直接进入 NodeLoc 收款金额 |
| 后台商品 | `GET /admin/products`、`GET /admin/products/:id` | `products:view` |
| | `POST` / `PUT` / `DELETE /admin/products[/:id]` | `products:manage` 写请求按整行走：创建时不写 `is_published` / `stock_visible` 仍是「上架 / 显示库存」，但显式写 `false` 就一定落成 `false`（这几列不再有数据库默认值，勾掉的开关不会再被悄悄写回打开）；`PUT` 是整行覆盖，前台表单要把 `is_published`、`stock_visible`、`is_featured`、`require_contact` 一并提交。`auto_deliver` 由商品类型决定，传了也不作数 |
| | `GET /admin/low-stock` | `products:view` 按后台设置的阈值列出待补货商品，每行带 `waiting_orders`：已付款、正在等这件商品卡密的订单笔数。队列先排欠单最多的（欠单相同再按货架最空），补货从收过钱的那一格开始 |
| | `POST /admin/low-stock/alert` | `cards:view` 立刻跑一轮缺货巡检，并把预警写进每位补货人的通知箱，回 `{"checked": 缺货件数, "sent": 这次新发出的条数, "threshold": 当前阈值}`。`checked` 只数「卖过或有人在等」的货架（低于阈值但从没开张的不算），所以这个数不会比补货队列里真正欠着的多；同一件商品对同一个人每个自然日只发一条（按商店本地时区算），再点一次 `sent` 是 0 而 `checked` 照旧——重复点击不会把收件箱刷满。收件人由权限决定：启用中且角色能看 `cards:view` 的账号才收，客服与普通买家收不到。后台清扫每 3 分钟跑的是同一轮（首轮在启动 20 秒后），这个按钮只是让人当场确认。提醒还没接上时返回 503 `alerts_unavailable`，而不是假装已经发出去了 |
| | `GET` / `POST` / `PUT` / `DELETE /admin/categories[/:id]` | `categories:view` / `categories:manage` 创建时不写 `is_visible` 仍是前台可见，写 `false` 就真的隐藏（前台分类列表与筛选都不再出现该分类） |
| 后台卡密 | `GET /admin/cards` | `cards:view` 跨商品视图，支持 `product_id`、`status`、`q`、分页 |
| | `GET /admin/cards/export` | `cards:manage` 带 BOM 的 UTF-8 CSV，Excel 直接双击可开 |
| | `GET /admin/products/:id/cards` | `cards:view` |
| | `POST /admin/products/:id/cards`、`.../cards/batch-add`、`.../cards/generate` | `cards:manage` 批量导入**不去重**：与该商品已有卡密相同的内容一样入库，一个卖同一张激活码的店就是每卖一份添一行；响应的 `duplicates` 只报「这次重复了几条」，`created` 仍是实际入库的条数（空白行会被丢掉并计入 `blank`）。内置生成器仍然避开随机撞车，一次批量生成的 key 互不相同。三个写入口都回 `released`：这次上货当场交付了几笔原本卡在「等待补货」的订单（新单张若一创建就停用则算 0，因为那不是补货） |
| | `POST .../cards/batch-status`、`.../cards/batch-delete`、`PUT` / `DELETE .../cards/:card_id` | `cards:manage` 批量删除只动未售出的卡，已售出的属于买家订单；`batch-status` 回 `{ "updated": 改了几张, "released": 当场发出几笔 }`，只有把卡放回货架（`available`）才会有 `released`，停用它是从架上取下 |
| 后台优惠码 | `GET /admin/coupons` | `coupons:view` |
| | `POST` / `PUT` / `DELETE /admin/coupons[/:id]` | `coupons:manage` `advertised` 决定这张码是否上前台促销架，创建时不写就是不公开（私下发给某个客户的码不会顺手公开）；`is_active` 不写仍是启用，写 `false` 就真的是停用；`PUT` 是整行覆盖，编辑时要带上 `advertised` 与 `is_active`，漏写等于关掉 |
| 后台订单 | `GET /admin/orders`、`GET /admin/orders/:order_no` | `orders:view` 列表支持 `status`、`user`、`q`、`attention=undelivered`（该参数取代 `status`）。后台地址栏用同一组键，另加 `page`（第几页，一页 20 笔），页码超出实际页数时自动退回最后一页 |
| | `GET /admin/orders/export` | `orders:view` 按同一组筛选参数导出 CSV（`status`、`q`、`user_id`、`attention`；不带分页，导的是整个筛选结果）。响应是 `text/csv` 附件、带 UTF-8 BOM，`X-Export-Rows` 报实际行数，超过 20000 笔时带 `X-Export-Truncated: 1` 并在文件末尾写明。金额写的就是店家定价、屏幕显示、向 NodeLoc 申请收款的那个整数（列名为 `单价(NL)` / `优惠(NL)` / `实付(NL)`，照抄数字，不补小数点也不做任何换算，与商品页和订单详情对得上），卡密只报发了几张、不含卡密内容；以 `=` `+` `-` `@` 开头的文本会先加保护再写入，防止在表格里被执行。参数非法（`user_id=-3`、未知的 `attention`）返回 400，不会被当成「没筛」 |
| | `POST /admin/orders/:order_no/cancel`、`/deliver`、`/refund`、`/fulfill`、`/reconcile`、`POST /admin/reconcile/pending` | `orders:manage` |
| 后台用户 | `GET /admin/users`、`GET /admin/users/:id` | `users:view` |
| | `POST /admin/users/:id/toggle-active`、`/points` | `users:manage` |
| | `POST /admin/users/:id/role`、`/toggle-admin` | `roles:manage` 改角色就是改权限，所以归到角色授权；且只有超级管理员能动后台账号 |
| 店家转账 | `POST /admin/users/:id/transfer` | `users:manage` 向该买家的 NodeLoc 账户转出 NL。请求体 `{amount, note}`：`amount` 必须是 1–1000 的整数，在本地就被拦下（`grant_amount` `400`），不打到 NodeLoc、不写流水；操作者永远取自会话，请求体里写谁都无用。收款账号不存在是 `grant_recipient_missing` `404`（「刷新用户列表后再试」），已停用是 `grant_recipient_inactive` `409`，没有绑定 NodeLoc 是 `grant_recipient_unknown` `409`。成功回 `{ "data": 流水行 }`，带 `reference`（`NLG…`，NodeLoc 用来去重的单号）、`to_user_id` / `to_username` / `amount` / `status`；被拒时回 `code` + 中文 `error`，`detail` 里附 NodeLoc 原文，八种拒绝各成码：`grant_insufficient_balance` `409`、`grant_disabled` `409`、`grant_recipient_mismatch` `409`、`grant_self` `409`、`grant_below_minimum` `400`、`grant_above_maximum` `400`、`grant_signature` `502`、`grant_reference_exists` `409`；状态含糊（`pending` 之类）与网关无回音按未完成处理，是 `grant_unanswered` / `provider_unreachable` `502`，文案会劝人先去流水确认而不是立刻重转。成功与失败都留一行流水，只有成功才给买家发通知；两者都写审计日志（动作 `user.transfer`，详情带金额、收款 uid 与流水号） |
| | `GET /admin/users/:id/transfers`、`GET /admin/transfers` | `users:view` 单个买家的流水 / 全店流水，倒序，`limit`（默认 50）与 `offset` 分页，回 `{ "data": [...], "total": N, "limit": L, "offset": O }`；`limit=0` 或负数是 `400`，不会被当成「没翻页」 |
| 通知 | `GET /notifications`、`POST /notifications/:id/read`、`GET /notifications/unread`、`POST /notifications/read-all` | 登录 只返回自己的通知；`unread` 数的是整个收件箱而不是当前页，`read-all` 一次清空并回报改动了多少条（再点一次回报 0，不谎报）。同一条重复标已读返回 200，不会因为「值没变」被判成找不到。订单的付款、发货、退款节点由商店自己写入买家收件箱（`type=order`、`link=/orders/单号`），买家只收到自己订单的消息。缺货预警写在能打开卡密页的账号名下（`type=stock`、`link=/cards/商品ID`，普通买家收不到）。店家转账成功后写给买家（`type=transfer`，文案带金额、流水号与店家留言；余额在 NodeLoc 而不在本店，所以这条不带站内链接。被 NodeLoc 拒掉的转账不发信，因为钱根本没出去）。`?type=promo`、`?unread=1` 可各自单用或叠用，`total` 跟着筛选走（筛出来多少就报多少），`kinds` 却始终数整个收件箱（每个类型的总数与未读数，按类型名排序），所以切到某一类也不会把「促销还有 3 条」读成「促销只剩 2 条」。`page_size` 上限 100、`page` 下限 1，响应里的 `page`/`page_size` 报的是实际生效值而不是请求原样——要让人翻页的接口不能把自己没用的参数回给人看。排序是 `created_at DESC, id DESC`，同一秒落地的多条消息也不会因为时间戳打平而在两页里重复出现或凭空消失 |
| | `POST /notifications`、`POST /admin/notifications/broadcast` | `notifications:manage` 定向发送 / 全量广播 |
| 看板与设置 | `GET /admin/stats` | `stats:view` 概览全部面板（含上期对比、优惠码成效、卡密健康）都走这一个接口。`stock_alerts[]` 每行带 `waiting`（已付款、在等这件商品卡密的订单笔数），面板先排 `waiting` 重的、再排货架空的 |
| | `GET /admin/settings` | `settings:view` 密钥以 `********` 回显，原样提交即保留原值。同时回报收款就绪状态：`payment_ready`（凭据是否齐到真能收款）与 `payment_missing`（缺哪几项，用设置页上的字段名：`payment_id` / `token` / `secret_key` / `base_url`）、「支付 API 地址」留空时下单/查单/转账跟随 OAuth 域名；登录侧同样回报 `oauth_ready` / `oauth_missing`（`client_id` / `client_secret` / `domain`）与 `oauth_warnings`（把 Client ID 填成 `pay_` 开头的 Payment ID、凭据里带着空格引号、回调地址的域名或路径与本店对不上——都只提醒，不拦保存）。「站点域名」会被归一成纯主机名（`HTTPS://Shop.Example.COM/x` → `shop.example.com`），回调地址手填时少了协议头也会自动补上本店的 `http`/`https` |
| | `GET /admin/oauth-attempts` | `settings:view` 最近买家登录的排查记录（`?limit=`，省略或 ≤0 取 20 条，上限 200，非数字 400），每条是 `step`（`initiate` 发起授权 / `callback` 回调）、`outcome`（`started` / `success` / `failed`）、`reason`（`disabled` / `not_configured` / `rejected` / `unreachable` / `provider` / `denied` / `expired` / `state` / `bind`）、`detail`（中文说明 + 服务商原文）、`redirect_uri`、`username`、`binding`。**授权码、`state`、Client Secret 与换回来的会话 token 一律不入库**（写盘前按正则洗一遍，超长正文裁到 500 字），所以这页可以给店家看，不会把一次登录的钥匙留在店里。这张表只留最近 500 条，写入时顺手裁掉更早的——它记的是买家名字，不该变成店里「谁登录过」的永久档案。设置页「最近 NodeLoc 登录记录」用的就是它 |
| | `PUT` / `POST /admin/settings`、`POST /admin/settings/oauth-test`、`/payment-test` | `settings:manage` 「测试 NodeLoc 登录」向 `/oauth-provider/token` 换一个不可能存在的授权码（只一个 POST，绝不碰 userinfo、不回显密钥），按 OAuth 错误码分别给中文结论并附 NodeLoc 原文：认这组凭据 / `client_rejected`（Client ID 与 Secret 对不上）/ `redirect_mismatch` / `grant_unsupported` / `scope_rejected` / `unreachable`（连不上）/ `route_missing`（这个域上没有 OAuth 接口——**商店自己的 SPA 对未知路径回 HTTP 200 + HTML，也算这一类**，不会被当成绿灯）/ `not_configured`。「测试支付网关」先用本店一笔**已成交订单**的 `order_id` 重放一次下单（NodeLoc 对同一 `order_id` 幂等，不会重复扣钱）来证明签名密钥，再敲查单，并把结果分成互不混为一谈的几类：`支付参数未配置完整，缺少 X`（参数问题，不怪网络）/ `无法连接支付网关: …`（连不上）/ `NodeLoc 拒绝了商店的签名…`（凭据不对，附实测过的四档）/ `还没有测出下单凭据是否可用`（`not_verified`：这家店还没有任何一单在 NodeLoc 留下交易号，按钮不肯凭空说绿灯）/ `支付网关可达，但查单只接受论坛后台的浏览器会话…`（`provider_guarded`，判定为**可达**）/ `支付网关连通正常…`（签名被接受）。除后两类都 `ok:false`；最坏情况整次自检不超过四个 POST |
| 图片素材 | `POST /admin/uploads/products`、`POST /admin/uploads/site` | `products:manage` / `settings:manage` 表单字段名 `image`。类型看文件头而不是扩展名，只收 png / jpg / gif（SVG 会被拒——那是一段有时会画图的脚本）；单张 2 MB、边长 8192 px 以内，超了分别 `413` / `400`。文件名由服务端生成，返回 `{url,width,height,size}`，`url` 就是「封面图」或「Logo 地址」该填的内容 |
| 权限目录 | `GET /admin/permissions`、`GET /admin/roles` | `roles:view` 目录由服务端给出，前端不写死资源清单；角色行里的 `user_count` 是该角色当前的账号数，保存前能看清改动影响到谁 |
| | `PUT /admin/roles/:role/permissions` | `roles:manage` `super_admin` 拒绝被修改，避免店家把自己锁在门外 |
| 审计 | `GET /admin/audit-logs` | `logs:view` 查询参数：`page`、`limit`（≤100）、`action`（前缀匹配，填 `order` 也能命中 `order.refund`）、`search`（在 `action` / `target` / `detail` 与操作者用户名里找包含关系，`%` 与 `_` 按字面处理）、`actor`（用户 ID，或 `system` 只看商店自己写下的记录）、`since` / `until`（`YYYY-MM-DD`，按商店本地时区的整天算，`until` 那天包含在内）。每条记录带 `actor_name`（一次批量查库解析本页的操作者，账号已注销也照样点名；查不到就不返回该字段，日志行本身不丢） |
| | `GET /admin/audit-logs/actions` | `logs:view` 本店已经出现过的操作名（去重、按字母序），日志页的输入框用它们做联想；清单来自历史，不是写死的目录 |
| | `GET /admin/audit-logs/export` | `logs:view` 按与列表同一组筛选参数导出 CSV（`action`、`search`、`actor`、`since`、`until`；不带分页，导的是整个筛选结果，最多 20000 条）。响应是 `text/csv` 附件、带 UTF-8 BOM，`X-Export-Rows` 报实际行数，被截断时带 `X-Export-Truncated: 1` 并在文件末尾写明。操作者写成用户名而不是 ID，以 `=` `+` `-` `@` 开头的文本先加保护再写入；参数不合式（`actor=abc`、`since=yesterday`、开始晚于结束）返回 400 + 中文说明，不会被当成「没筛」 |

### 各角色的默认授权

首次安装即按下表播种，之后店家可在 **后台 → 角色** 自由增删；这些只是默认值，不是硬编码的天花板。

| 角色 | 默认能做什么 |
|---|---|
| 超级管理员 | 全部。唯一能授予「管理员 / 超级管理员」、调整后台账号角色的角色，本身不可被编辑 |
| 管理员 | 商品、卡密、分类、优惠码、订单、用户、通知、设置的查看 + 管理，看板、审计日志，以及角色查看与授权 |
| 运营 | 商品 / 卡密 / 分类 / 优惠码 / 订单的查看与管理，看板；**不**碰用户、通知发送与设置 |
| 客服 | 订单查看 + 处理（发货、补发、对账）、用户与通知查看、看板；**不**碰商品、卡密、优惠码、设置与角色 |
| 普通用户 | 只有前台：下单、订单、个人中心 |

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
├── cmd/server/            # Go 入口（swapHandler 热重建 + SPA 静态托管 + 文档头部改写 + 上传路由）
├── internal/
│   ├── config/            # 默认值 + data/bootstrap.json（驱动/DSN/端口/JWT）
│   ├── app/container/     # 依赖装配
│   ├── app/httpserver/    # JWT、Casbin 授权中间件、审计
│   ├── authz/             # Casbin RBAC
│   ├── models/            # 共享 GORM 模型与迁移
│   ├── platform/
│   │   ├── database/      # SQLite / MySQL 接入
│   │   └── upload/        # 后台图片：认字节、限大小、由服务端命名
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
| 下单报「支付还没有配置好」 | 后台 **设置** 的 Payment ID 必填，签名密钥**填 Payment Token 或 Secret Key 任意一格即可**（NodeLoc 的支付应用有的发两串、有的只发一串；只有一串时填进 Secret Key 更稳妥，商店会依次试文档写的几种签名方式）。填好后点「测试支付网关」，它会真签一次请求并把实测到的那一档写在结论里。设置接口把状态写在 `payment_ready` / `payment_missing` / `payment_warnings` 里，后台页面据此显示「还收不了款」并点名缺哪一项，或对填了但不可能对的组合（Token 与 Secret Key 同串、Payment ID 填成了 OAuth Client ID、密钥还是 `********`）给出提醒 |
| 「测试支付网关」说查单调不动 | 这不是故障，但这句话**是有前提的**：商店先用本店一笔已成交订单的 `order_id` 重放一次下单，NodeLoc 回「订单已存在」证明了签名密钥，才允许把查单的 `403 ["BAD CSRF"]` 说成「可达」。NodeLoc 的 `POST /payment/query/{payment_id}` 是论坛后台的浏览器路由，服务器端的调用一律被 Discourse 的会话闸门挡下，商店改走两条服务端真能调通的路径：浏览器回调验签，以及把同一 `order_id` 再发一次下单读回 NodeLoc 的状态词。收款、发货、对账、退款都不受影响，不需要动凭据。这一降级只维持 **15 分钟**：窗口过后商店会自动重敲一次查单，NodeLoc 哪天放开成服务器接口就自动用回它；而网关偶发吐出的 HTML 页（哪怕是 HTTP 200）只让**那一次**调用改道，不会把路由整条摘掉。后台的对账结果会标明这一单「经由下单核实」及其原因，买家侧看不到这些内部信息 |
| 「测试支付网关」说「还没有测出下单凭据是否可用」 | `not_verified`：新装的店一笔真实订单都还没有成交，而查单在真 NodeLoc 上调不动，于是没有任何一条路能在**不打开新支付**的前提下证明这把密钥对不对——商店不肯凭空给绿灯。先去下一单并自己付掉一笔（或等买家付成一单），再按这个按钮即可。此时设置页的收款就绪状态仍按参数齐不齐来判断，这一条只说明「还没实测过」 |
| 「测试支付网关」说 IP 不在白名单 | `provider_ip_blocked`（编号 `1003`）：NodeLoc 的支付应用可以限定只接受某些服务器 IP。这一关在论坛读你的密钥**之前**就跑完，所以「签名不对」的提示对你没意义，改 Token / Secret Key、重建容器都不会有变化。把商店服务器的公网 IP 加进 <https://www.nodeloc.com/payment/applications> 那个应用的白名单；容器换过宿主、IP 随之改变时，症状就是「凭据明明对，每条路由都被拒」 |
| 「测试支付网关」说找不到这个 Payment ID | `payment_id_unknown`：支付应用答了 404 而不是拒绝签名。先看后台订单上那句「NodeLoc 原文」怎么说的——**空正文的 404**（实测 NodeLoc 对未知编号就是这样回的）说「这个 Payment ID 没有对应的支付应用」，去 <https://www.nodeloc.com/payment/applications> 复制 `pay_` 开头的那串编号，它和 OAuth 登录用的 Client ID 是两个不同的值，混填会让每个买家都在下单时被拒；带回「论坛 404 网页」的说「这个域上根本没有支付接口的路由」，那是「支付 API 地址」指向了没装支付应用的域（常见于填成登录镜像域）。两种都只发一次请求，也不会被记成「这条路不通」 |
| 每个买家都只看见「无法支付」，参数又确实填对了 | 核对 **设置 → 支付 API 地址**：留空时下单/查单/转账跟随 OAuth 域名。登录走镜像域而支付走主域的店，请求会发到没有支付应用的域上，每一单都失败。这种情况下重复点击没有用，商店给买家的文案会写成「这是商店与 NodeLoc 之间的问题…请把订单号发给店家处理」而不是「稍后再试」。另外，付款失败时订单已经建好了：详情页上的「再试一次支付」用原单重发，不会重复下第二单 |
| 转账给买家被拒 | 后台会把 NodeLoc 的八种拒绝各说成一句话并带上数字：余额不够（写出需要多少、现有多少）/ 支付应用没开转账功能 / 收款方 uid 与用户名对不上（让对方重新用 NodeLoc 登录一次以更新绑定）/ 不能给自己转 / 低于或超过限额（限额写在文案里）/ 签名无效 / 单号已被收过（先去流水确认，别重复转出）。每一种都照样进流水（`status: failed`，带原文），只有成功才给买家发通知。状态含糊或网关无回音按未完成处理（`grant_unanswered`），提示先去流水核对而不是立刻重转。**余额、限额、开关、收款方这些业务拒绝只发一次请求**：NodeLoc 能回答这一笔业务就说明密钥已经被接受，商店不会在扣钱的接口上换密钥重跑四次，并把这一档签名记下来给后续调用 |
| 回调签名验证失败 | 官方文档在两处把回调密钥分别写成 **Secret Key 原文**和 `hex(SHA256(Payment Token))`，商店两种都验（再算上 Secret Key 的 SHA-256 与 Token 原文，共四种拼法），任一命中即认账；跳转里缺 `transaction_id` 也能入账，商店会用该单原本存下的交易号补上。仍对不上时不会直接判定「没付款」：商店会自动用下单回执向 NodeLoc 核实这一单，已付就当场入账发卡，无需重复付款 |
| 下单/查单/转账全部被拒，凭据又确实填对 | 核对宿主机时间：NodeLoc 要求每个请求带 10 位秒级 `timestamp`，与服务器相差超过 5 分钟就拒绝。同步 NTP 即可，支付凭据不用动。「测试支付网关」碰到这类拒绝会直接说是时钟问题 |
| 同一个订单点第二次「去支付」 | 不会再开第二笔收款。NodeLoc 说这一单「Order already exists with status …」时，商店按它的状态处理：已付 → 当场查单入账发卡；未付 → 返回这一单原本的付款地址 |
| OAuth 登录未完成 | 登录页会把原因写清楚：**本店还没配齐**（`not_configured`，点名缺哪一项，商店其余部分不受影响）/ **连不上 NodeLoc**（`unreachable`）/ **NodeLoc 拒绝**（`rejected`，换 token 失败，核对 Client ID/Secret 与回调白名单）/ **授权被拒绝**（用户在 NodeLoc 点了取消）/ **链接已过期**（回调没带上本浏览器的 `state`，常见于复制链接、隔了很久再打开、或 Cookie 被拦）/ **服务商异常**（NodeLoc 本身报错）。中间三类重试即可；其余核对后台设置。**「NodeLoc 拒绝」到底拒了什么，服务端说人话**：实测 token 端点用标准 OAuth 错误码回答（凭据不对 → `400 {"error":"invalid_client","error_description":"Invalid client credentials"}`；`/oauth-provider/userinfo` 无 token → `401 {"error":"invalid_token"}`），商店把这些码翻成「Client ID 与 Client Secret 对不上这个 OAuth 应用（两串填反、应用被重建或 Secret 被重置都会这样）」「授权码已经用过或已经过期，让买家重新点一次」这样的中文，并始终附上 NodeLoc 原文；回的是网页而不是 JSON 时说的是「这个域上没有 OAuth 应用，或请求没走到论坛的扩展」，而不是让店家去换一对本来正确的密钥 |
| 「测试 NodeLoc 登录」该怎么信 | 它**不再靠拼授权链接判断**——论坛对 `/oauth-provider/authorize` 无论 Client ID 真假都 302 到它自己的登录页，能拼出 URL 什么都证明不了，凭据被重置/填错的店过去在这里读到绿灯、买家却在登录后处处碰壁。现在它向 `/oauth-provider/token` 换一个**不可能存在的授权码**（只一个 POST，不碰任何账号、不回显密钥），按 OAuth 自己的错误码判定：`ok:true` 表示论坛认这对 Client ID/Secret；`client_rejected` 是这对对不上（去应用页重新复制，别把 Payment ID 填进 Client ID）；`redirect_mismatch` 是回调白名单与商店报的 `/api/v1/auth/oauth/callback` 一字不差的差一点；`grant_unsupported`/`scope_rejected` 是应用没勾授权码模式或没放 openid；`unreachable` 是网络/域名问题；`route_missing` 是「这个地址上根本没有 NodeLoc 的 OAuth 接口」——**把『NodeLoc 站点地址』填成了商店自己的域名就会落在这里**（商店 SPA 对未知路径回 HTTP 200 + HTML，光看 2xx 会被骗成绿灯，商店读正文形状才认）。结论用中文，NodeLoc 原文附在后面 |
| 邮件没拿到 | NodeLoc OAuth `email` scope 需审核通过；未通过时 token 只有 `openid` |

| 前台确认支付结果不通过 | 页面会给出具体原因码（见 2.4）：`unsettled` / `provider_unreachable` 属可重试，稍后再查即可；`no_transaction` 表示这单根本没到 NodeLoc，要点「继续支付」重新发起；`amount_mismatch` / `foreign_transaction` 会停止自动入账，只能店家核对。后台日志里同一笔会带上 NodeLoc 的原文 |
| 卡密一直没发货 | 补货的接口会当场放出这一单的卡密并在响应里回 `released`，后台提示也写成「已自动发出 N 笔等待中的订单」，所以正常情况下不必等清扫。先看订单列表的「需处理交付」队列：`manual_pending` 需要店家点「标记已发货」；`waiting_stock` 说明补进去的货不够这一单（或那几张卡被停用了），核对 Admin → 卡密 里该商品的可用数与「N 笔已付款在等」，再查 Admin → 日志 的 `payment.stock_warning`。3 分钟一轮的清扫仍然兜底，放队失败的原因写在服务器日志里 |
| 升级后概览或角色页 403 | 不用手工补：容器启动时会把授权目录升到当前版本（旧的 `dashboard:view` 自动迁成 `stats:view`，并补齐各角色缺失的那几项），已在**后台 → 角色**里主动关掉的权利不会被重新打开 |
| 某个后台账号看不见某个页面 | 后台所有按钮都以 `GET /auth/me/permissions` 为准。让超级管理员到 **角色** 里给该角色勾上对应资源，或直接看 403 响应里的 `permission` 字段缺哪一项 |
| 老店没有超级管理员，加不了第二个管理员 | 早期向导把首个账号建成「管理员」，而授予管理员只属于超级管理员。现在的构建启动时会检查：库里没有任何超级管理员时，把**最早那个启用的管理员**升为超级管理员（日志 `[authz] no super_admin in store; promoted user …`）。已经有一个超级管理员后这个检查不会再动任何角色，主动降级不会被偷偷改回来 |
| 重启后丢失初始化状态 | `./data` 没挂载持久卷，按 Step 3 补上 `-v "$PWD/data:/app/data"` 重新起 |
| 上传图片 413 | OpenResty 的 `client_max_body_size` 与应用一致（默认 8M） |

## 📜 License

MIT

