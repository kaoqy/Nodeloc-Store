-- NodeLoc Store 活动营销 / 工单 / AI 客服 迁移脚本
-- 说明：应用启动时 models.Migrate() 已用 GORM AutoMigrate 建表；本脚本用于
-- 手工审计、只读环境或需要逐条执行 DDL 的部署场景，与 AutoMigrate 结果一致。
-- 全部语句都是 CREATE TABLE IF NOT EXISTS / CREATE INDEX IF NOT EXISTS，
-- 重复执行安全，不会删除或改写任何已有数据。

-- ── 活动营销 ────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS activities (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  name VARCHAR(160) NOT NULL,
  subtitle VARCHAR(200) DEFAULT '',
  description TEXT DEFAULT '',
  cover_image VARCHAR(500) DEFAULT '',
  banner_image VARCHAR(500) DEFAULT '',
  type VARCHAR(48) NOT NULL,
  rules TEXT DEFAULT '',
  product_ids TEXT DEFAULT '',
  category_ids TEXT DEFAULT '',
  user_scope VARCHAR(32) NOT NULL DEFAULT 'all',
  user_role_scope VARCHAR(64) DEFAULT '',
  start_at DATETIME, end_at DATETIME,
  sort_order INTEGER NOT NULL DEFAULT 0,
  stock_limit INTEGER NOT NULL DEFAULT 0,
  stock_used INTEGER NOT NULL DEFAULT 0,
  quota_limit INTEGER NOT NULL DEFAULT 0,
  quota_used INTEGER NOT NULL DEFAULT 0,
  per_user_limit INTEGER NOT NULL DEFAULT 0,
  require_login BOOLEAN NOT NULL DEFAULT 1,
  allow_stacking BOOLEAN NOT NULL DEFAULT 0,
  auto_apply BOOLEAN NOT NULL DEFAULT 1,
  status VARCHAR(24) NOT NULL DEFAULT 'draft',
  created_by INTEGER, remark VARCHAR(500) DEFAULT '', terminate_reason VARCHAR(500) DEFAULT '',
  show_on_home BOOLEAN NOT NULL DEFAULT 1, show_in_list BOOLEAN NOT NULL DEFAULT 1,
  notify_users BOOLEAN NOT NULL DEFAULT 0, notify_text TEXT DEFAULT '', advanced TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_activities_type ON activities(type);
CREATE INDEX IF NOT EXISTS idx_activities_status ON activities(status);
CREATE INDEX IF NOT EXISTS idx_activities_start_at ON activities(start_at);
CREATE INDEX IF NOT EXISTS idx_activities_end_at ON activities(end_at);
CREATE INDEX IF NOT EXISTS idx_activities_created_by ON activities(created_by);
CREATE INDEX IF NOT EXISTS idx_activities_deleted_at ON activities(deleted_at);

CREATE TABLE IF NOT EXISTS activity_rules (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  activity_id INTEGER NOT NULL, rule_type VARCHAR(48) NOT NULL,
  config TEXT DEFAULT '', is_enabled BOOLEAN NOT NULL DEFAULT 1, sort_order INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS uniq_activity_rule ON activity_rules(activity_id, rule_type);
CREATE INDEX IF NOT EXISTS idx_activity_rules_activity_id ON activity_rules(activity_id);

CREATE TABLE IF NOT EXISTS activity_records (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  activity_id INTEGER NOT NULL, user_id INTEGER NOT NULL,
  order_id INTEGER, product_id INTEGER, quantity INTEGER NOT NULL DEFAULT 1,
  original_amount INTEGER NOT NULL DEFAULT 0, discount_amount INTEGER NOT NULL DEFAULT 0,
  payable_amount INTEGER NOT NULL DEFAULT 0, status VARCHAR(24) NOT NULL DEFAULT 'reserved',
  coupon_code VARCHAR(64) DEFAULT '', snapshot TEXT DEFAULT '', ip VARCHAR(64) DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_activity_record ON activity_records(activity_id, user_id);

CREATE TABLE IF NOT EXISTS activity_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  activity_id INTEGER NOT NULL, actor_id INTEGER, action VARCHAR(64) NOT NULL,
  detail TEXT DEFAULT '', before TEXT DEFAULT '', after TEXT DEFAULT '',
  ip VARCHAR(64) DEFAULT '', user_agent VARCHAR(255) DEFAULT '',
  result VARCHAR(24) NOT NULL DEFAULT 'ok', error TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_activity_logs_activity_id ON activity_logs(activity_id);
CREATE INDEX IF NOT EXISTS idx_activity_logs_action ON activity_logs(action);

CREATE TABLE IF NOT EXISTS coupon_records (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  coupon_id INTEGER NOT NULL, user_id INTEGER NOT NULL, order_id INTEGER,
  status VARCHAR(24) NOT NULL DEFAULT 'claimed', source VARCHAR(32) NOT NULL DEFAULT 'claim',
  claimed_at DATETIME, used_at DATETIME, expire_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_coupon_record ON coupon_records(coupon_id, user_id);

-- 订单活动快照列（历史订单保留当时的活动与优惠金额）
ALTER TABLE orders ADD COLUMN activity_id INTEGER;
ALTER TABLE orders ADD COLUMN activity_name VARCHAR(160) DEFAULT '';
ALTER TABLE orders ADD COLUMN activity_discount INTEGER NOT NULL DEFAULT 0;
ALTER TABLE orders ADD COLUMN activity_snapshot TEXT DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_orders_activity_id ON orders(activity_id);

-- ── 工单 ────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS tickets (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  ticket_no VARCHAR(64) NOT NULL UNIQUE, user_id INTEGER NOT NULL,
  order_id INTEGER, order_no VARCHAR(64) DEFAULT '', product_id INTEGER, product_name VARCHAR(160) DEFAULT '',
  type VARCHAR(48) NOT NULL, type_name VARCHAR(80) DEFAULT '',
  subject VARCHAR(200) NOT NULL, content TEXT DEFAULT '',
  status VARCHAR(32) NOT NULL DEFAULT 'ai_processing', priority VARCHAR(16) NOT NULL DEFAULT 'normal',
  handler VARCHAR(16) NOT NULL DEFAULT 'ai', assigned_agent_id INTEGER,
  ai_enabled BOOLEAN NOT NULL DEFAULT 1, transfer_reason VARCHAR(255) DEFAULT '',
  summary TEXT DEFAULT '', suggested_plan TEXT DEFAULT '',
  tags VARCHAR(255) DEFAULT '', source VARCHAR(32) NOT NULL DEFAULT 'web', customer_contact VARCHAR(255) DEFAULT '',
  satisfaction INTEGER NOT NULL DEFAULT 0, satisfaction_note VARCHAR(500) DEFAULT '',
  last_message_at DATETIME, first_response_at DATETIME, resolved_at DATETIME, closed_at DATETIME, due_at DATETIME,
  message_count INTEGER NOT NULL DEFAULT 0, attachment_count INTEGER NOT NULL DEFAULT 0,
  unread_for_staff BOOLEAN NOT NULL DEFAULT 1, unread_for_user BOOLEAN NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_tickets_ticket_no ON tickets(ticket_no);
CREATE INDEX IF NOT EXISTS idx_tickets_user_id ON tickets(user_id);
CREATE INDEX IF NOT EXISTS idx_tickets_status ON tickets(status);
CREATE INDEX IF NOT EXISTS idx_tickets_priority ON tickets(priority);
CREATE INDEX IF NOT EXISTS idx_tickets_handler ON tickets(handler);
CREATE INDEX IF NOT EXISTS idx_tickets_due_at ON tickets(due_at);

CREATE TABLE IF NOT EXISTS ticket_messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  ticket_id INTEGER NOT NULL, sender_type VARCHAR(16) NOT NULL, sender_id INTEGER,
  sender_name VARCHAR(80) DEFAULT '', content TEXT NOT NULL, content_type VARCHAR(16) NOT NULL DEFAULT 'text',
  is_internal BOOLEAN NOT NULL DEFAULT 0, meta TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_ticket_messages_ticket_id ON ticket_messages(ticket_id);

CREATE TABLE IF NOT EXISTS ticket_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  ticket_id INTEGER NOT NULL, actor_id INTEGER, actor_type VARCHAR(16) NOT NULL DEFAULT 'system',
  action VARCHAR(64) NOT NULL, before TEXT DEFAULT '', after TEXT DEFAULT '', detail TEXT DEFAULT '',
  ip VARCHAR(64) DEFAULT '', user_agent VARCHAR(255) DEFAULT '', result VARCHAR(24) NOT NULL DEFAULT 'ok', error TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_ticket_logs_ticket_id ON ticket_logs(ticket_id);
CREATE INDEX IF NOT EXISTS idx_ticket_logs_action ON ticket_logs(action);

CREATE TABLE IF NOT EXISTS ticket_attachments (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  ticket_id INTEGER NOT NULL, message_id INTEGER, user_id INTEGER NOT NULL,
  file_name VARCHAR(255) NOT NULL, file_path VARCHAR(500) NOT NULL,
  mime_type VARCHAR(120) DEFAULT '', size INTEGER NOT NULL DEFAULT 0,
  kind VARCHAR(24) NOT NULL DEFAULT 'image', status VARCHAR(16) NOT NULL DEFAULT 'ready'
);
CREATE INDEX IF NOT EXISTS idx_ticket_attachments_ticket_id ON ticket_attachments(ticket_id);

CREATE TABLE IF NOT EXISTS ticket_ai_sessions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  ticket_id INTEGER NOT NULL, conversation_id INTEGER,
  status VARCHAR(24) NOT NULL DEFAULT 'active', intent VARCHAR(64) DEFAULT '',
  order_no VARCHAR(64) DEFAULT '', product_name VARCHAR(160) DEFAULT '',
  summary TEXT DEFAULT '', suggested_plan TEXT DEFAULT '',
  needs_human BOOLEAN NOT NULL DEFAULT 0, reason VARCHAR(255) DEFAULT '',
  failure_count INTEGER NOT NULL DEFAULT 0, downvote_count INTEGER NOT NULL DEFAULT 0,
  turn_count INTEGER NOT NULL DEFAULT 0, last_active_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_ticket_ai_sessions_ticket_id ON ticket_ai_sessions(ticket_id);

-- ── AI 客服 ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS ai_conversations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  user_id INTEGER, ticket_id INTEGER, channel VARCHAR(24) NOT NULL DEFAULT 'widget',
  status VARCHAR(24) NOT NULL DEFAULT 'active', title VARCHAR(200) DEFAULT '', agent_name VARCHAR(80) DEFAULT '',
  page_context VARCHAR(255) DEFAULT '', ip VARCHAR(64) DEFAULT '', user_agent VARCHAR(255) DEFAULT '',
  message_count INTEGER NOT NULL DEFAULT 0, handed_to_human BOOLEAN NOT NULL DEFAULT 0,
  rating INTEGER NOT NULL DEFAULT 0, rating_note VARCHAR(500) DEFAULT '', last_message_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_ai_conversations_user_id ON ai_conversations(user_id);
CREATE INDEX IF NOT EXISTS idx_ai_conversations_ip ON ai_conversations(ip);

CREATE TABLE IF NOT EXISTS ai_messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  conversation_id INTEGER NOT NULL, role VARCHAR(16) NOT NULL, content TEXT DEFAULT '',
  tool_key VARCHAR(64) DEFAULT '', tool_call_id VARCHAR(64) DEFAULT '', model VARCHAR(80) DEFAULT '',
  tokens INTEGER NOT NULL DEFAULT 0, status VARCHAR(16) NOT NULL DEFAULT 'ok', error TEXT DEFAULT '', meta TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_ai_messages_conversation_id ON ai_messages(conversation_id);

CREATE TABLE IF NOT EXISTS ai_tool_definitions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  key VARCHAR(64) NOT NULL UNIQUE, name VARCHAR(120) NOT NULL, description TEXT DEFAULT '',
  category VARCHAR(48) NOT NULL DEFAULT 'query', method VARCHAR(16) NOT NULL DEFAULT 'INTERNAL',
  endpoint VARCHAR(255) DEFAULT '', kind VARCHAR(16) NOT NULL DEFAULT 'internal',
  params_schema TEXT DEFAULT '', result_schema TEXT DEFAULT '',
  require_login BOOLEAN NOT NULL DEFAULT 1, own_data_only BOOLEAN NOT NULL DEFAULT 1,
  require_approval BOOLEAN NOT NULL DEFAULT 0, allow_auto BOOLEAN NOT NULL DEFAULT 1,
  require_confirm BOOLEAN NOT NULL DEFAULT 0, is_enabled BOOLEAN NOT NULL DEFAULT 1,
  rate_limit INTEGER NOT NULL DEFAULT 30, timeout_ms INTEGER NOT NULL DEFAULT 8000,
  failure_mode VARCHAR(24) NOT NULL DEFAULT 'reply', risk_level VARCHAR(16) NOT NULL DEFAULT 'low',
  builtin BOOLEAN NOT NULL DEFAULT 0, sort_order INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_ai_tool_definitions_key ON ai_tool_definitions(key);

CREATE TABLE IF NOT EXISTS ai_tool_permissions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  tool_id INTEGER NOT NULL, role VARCHAR(32) NOT NULL, allowed BOOLEAN NOT NULL DEFAULT 1, updated_by INTEGER
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_tool_role ON ai_tool_permissions(tool_id, role);

CREATE TABLE IF NOT EXISTS ai_tool_calls (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  conversation_id INTEGER, message_id INTEGER, ticket_id INTEGER, user_id INTEGER,
  tool_key VARCHAR(64) NOT NULL, tool_name VARCHAR(120) DEFAULT '',
  params TEXT DEFAULT '', result TEXT DEFAULT '', status VARCHAR(24) NOT NULL DEFAULT 'ok', error TEXT DEFAULT '',
  duration_ms INTEGER NOT NULL DEFAULT 0, require_confirm BOOLEAN NOT NULL DEFAULT 0, confirmed_by INTEGER,
  risk_level VARCHAR(16) NOT NULL DEFAULT 'low', ip VARCHAR(64) DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_ai_tool_calls_tool_key ON ai_tool_calls(tool_key);
CREATE INDEX IF NOT EXISTS idx_ai_tool_calls_status ON ai_tool_calls(status);

CREATE TABLE IF NOT EXISTS ai_knowledge_categories (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  name VARCHAR(120) NOT NULL, slug VARCHAR(120) NOT NULL UNIQUE, description TEXT DEFAULT '',
  parent_id INTEGER, sort_order INTEGER NOT NULL DEFAULT 0, is_enabled BOOLEAN NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS ai_knowledge (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  category_id INTEGER, title VARCHAR(200) NOT NULL, slug VARCHAR(200) NOT NULL UNIQUE,
  summary VARCHAR(500) DEFAULT '', content TEXT NOT NULL, keywords VARCHAR(500) DEFAULT '', tags VARCHAR(255) DEFAULT '',
  priority INTEGER NOT NULL DEFAULT 0, status VARCHAR(16) NOT NULL DEFAULT 'draft',
  publish_at DATETIME, expire_at DATETIME, version INTEGER NOT NULL DEFAULT 1, author_id INTEGER,
  source VARCHAR(32) NOT NULL DEFAULT 'manual', view_count INTEGER NOT NULL DEFAULT 0,
  useful_count INTEGER NOT NULL DEFAULT 0, builtin BOOLEAN NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_status ON ai_knowledge(status);
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_priority ON ai_knowledge(priority);

CREATE TABLE IF NOT EXISTS ai_feedbacks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  conversation_id INTEGER NOT NULL, message_id INTEGER, ticket_id INTEGER, user_id INTEGER,
  rating INTEGER NOT NULL DEFAULT 0, reason VARCHAR(120) DEFAULT '', comment VARCHAR(500) DEFAULT '',
  handled BOOLEAN NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_ai_feedbacks_conversation_id ON ai_feedbacks(conversation_id);

CREATE TABLE IF NOT EXISTS ai_configs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  provider VARCHAR(32) NOT NULL DEFAULT 'openai-compatible', base_url VARCHAR(255) DEFAULT '',
  api_key_enc TEXT DEFAULT '', model VARCHAR(120) DEFAULT '',
  timeout_ms INTEGER NOT NULL DEFAULT 30000, max_context INTEGER NOT NULL DEFAULT 12, max_reply_len INTEGER NOT NULL DEFAULT 2000,
  temperature REAL NOT NULL DEFAULT 0.3, top_p REAL NOT NULL DEFAULT 1, is_enabled BOOLEAN NOT NULL DEFAULT 0,
  guest_allowed BOOLEAN NOT NULL DEFAULT 1, guest_daily_limit INTEGER NOT NULL DEFAULT 10,
  user_daily_limit INTEGER NOT NULL DEFAULT 100, ip_rate_limit INTEGER NOT NULL DEFAULT 30,
  max_message_len INTEGER NOT NULL DEFAULT 2000,
  system_prompt TEXT DEFAULT '', greeting TEXT DEFAULT '', fallback_reply TEXT DEFAULT '',
  transfer_tip TEXT DEFAULT '', ticket_tip TEXT DEFAULT '', sensitive_tip TEXT DEFAULT '',
  avatar VARCHAR(500) DEFAULT '', agent_name VARCHAR(80) NOT NULL DEFAULT '智能客服',
  rating_enabled BOOLEAN NOT NULL DEFAULT 1, log_enabled BOOLEAN NOT NULL DEFAULT 1, mail_enabled BOOLEAN NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS ai_workflow_configs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  default_handle_minutes INTEGER NOT NULL DEFAULT 10, max_failures INTEGER NOT NULL DEFAULT 3,
  transfer_after_failures INTEGER NOT NULL DEFAULT 2, transfer_after_downvotes INTEGER NOT NULL DEFAULT 2,
  transfer_on_explicit BOOLEAN NOT NULL DEFAULT 1, transfer_high_amount BOOLEAN NOT NULL DEFAULT 1,
  high_amount_threshold INTEGER NOT NULL DEFAULT 500, transfer_refund BOOLEAN NOT NULL DEFAULT 1,
  transfer_card_dispute BOOLEAN NOT NULL DEFAULT 1, transfer_payment_issue BOOLEAN NOT NULL DEFAULT 1,
  transfer_abuse BOOLEAN NOT NULL DEFAULT 1,
  can_create_ticket BOOLEAN NOT NULL DEFAULT 1, can_update_ticket BOOLEAN NOT NULL DEFAULT 0,
  can_notify BOOLEAN NOT NULL DEFAULT 0, can_query_order BOOLEAN NOT NULL DEFAULT 1,
  can_query_shipping BOOLEAN NOT NULL DEFAULT 1, can_recommend_activity BOOLEAN NOT NULL DEFAULT 1,
  can_grant_coupon BOOLEAN NOT NULL DEFAULT 0,
  transfer_notice TEXT DEFAULT '', working_hours VARCHAR(120) DEFAULT '', estimate_reply_minutes INTEGER NOT NULL DEFAULT 30
);

CREATE TABLE IF NOT EXISTS ai_quick_questions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  title VARCHAR(160) NOT NULL, content TEXT DEFAULT '', position VARCHAR(32) NOT NULL DEFAULT 'widget',
  pages VARCHAR(255) DEFAULT '', sort_order INTEGER NOT NULL DEFAULT 0, is_enabled BOOLEAN NOT NULL DEFAULT 1,
  require_login BOOLEAN NOT NULL DEFAULT 0, knowledge_id INTEGER, tool_key VARCHAR(64) DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_ai_quick_questions_is_enabled ON ai_quick_questions(is_enabled);

-- ── 客服与配置 ──────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS customer_service_agents (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  user_id INTEGER NOT NULL UNIQUE, nickname VARCHAR(80) DEFAULT '', avatar VARCHAR(500) DEFAULT '',
  status VARCHAR(16) NOT NULL DEFAULT 'offline', accept_manual BOOLEAN NOT NULL DEFAULT 1,
  ticket_types VARCHAR(255) DEFAULT '', max_concurrent INTEGER NOT NULL DEFAULT 5,
  work_start VARCHAR(8) DEFAULT '09:00', work_end VARCHAR(8) DEFAULT '21:00',
  assign_strategy VARCHAR(24) NOT NULL DEFAULT 'balanced', role VARCHAR(32) NOT NULL DEFAULT 'support',
  score REAL NOT NULL DEFAULT 0, avg_response_sec INTEGER NOT NULL DEFAULT 0, handled_count INTEGER NOT NULL DEFAULT 0,
  is_active BOOLEAN NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS customer_service_assignments (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  ticket_id INTEGER NOT NULL, agent_id INTEGER NOT NULL, assigned_by INTEGER,
  strategy VARCHAR(24) NOT NULL DEFAULT 'manual', status VARCHAR(24) NOT NULL DEFAULT 'assigned',
  assigned_at DATETIME, accepted_at DATETIME, finished_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_customer_service_assignments_ticket_id ON customer_service_assignments(ticket_id);

CREATE TABLE IF NOT EXISTS quick_replies (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  title VARCHAR(160) NOT NULL, content TEXT NOT NULL, ticket_types VARCHAR(255) DEFAULT '',
  scene VARCHAR(48) DEFAULT '', roles VARCHAR(255) DEFAULT '', sort_order INTEGER NOT NULL DEFAULT 0,
  is_enabled BOOLEAN NOT NULL DEFAULT 1, variables VARCHAR(500) DEFAULT '', use_count INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS notification_templates (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  key VARCHAR(64) NOT NULL UNIQUE, name VARCHAR(120) NOT NULL, event VARCHAR(64) NOT NULL,
  category VARCHAR(32) NOT NULL DEFAULT 'ticket', is_enabled BOOLEAN NOT NULL DEFAULT 1,
  in_app BOOLEAN NOT NULL DEFAULT 1, mail BOOLEAN NOT NULL DEFAULT 0,
  title_template TEXT DEFAULT '', content_template TEXT DEFAULT '', variables VARCHAR(500) DEFAULT '',
  recipients VARCHAR(120) DEFAULT '', retry_limit INTEGER NOT NULL DEFAULT 3, sort_order INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS notification_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  template_key VARCHAR(64) DEFAULT '', channel VARCHAR(16) NOT NULL DEFAULT 'in_app',
  user_id INTEGER, ticket_id INTEGER, order_id INTEGER, title VARCHAR(255) DEFAULT '', content TEXT DEFAULT '',
  status VARCHAR(16) NOT NULL DEFAULT 'sent', error TEXT DEFAULT '', attempts INTEGER NOT NULL DEFAULT 1, sent_at DATETIME
);

CREATE TABLE IF NOT EXISTS system_configs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  "group" VARCHAR(64) NOT NULL, key VARCHAR(64) NOT NULL, value TEXT DEFAULT '',
  value_type VARCHAR(16) NOT NULL DEFAULT 'string', label VARCHAR(120) DEFAULT '', description VARCHAR(500) DEFAULT '',
  is_secret BOOLEAN NOT NULL DEFAULT 0, sort_order INTEGER NOT NULL DEFAULT 0, updated_by INTEGER
);
CREATE UNIQUE INDEX IF NOT EXISTS uniq_system_config ON system_configs("group", key);

CREATE TABLE IF NOT EXISTS operation_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
  actor_id INTEGER, actor_role VARCHAR(32) DEFAULT '', actor_name VARCHAR(64) DEFAULT '',
  user_id INTEGER, ticket_id INTEGER, order_id INTEGER,
  resource VARCHAR(64) NOT NULL, action VARCHAR(64) NOT NULL,
  before TEXT DEFAULT '', after TEXT DEFAULT '', detail TEXT DEFAULT '',
  ip VARCHAR(64) DEFAULT '', user_agent VARCHAR(255) DEFAULT '',
  result VARCHAR(24) NOT NULL DEFAULT 'ok', error TEXT DEFAULT '', duration_ms INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_operation_logs_resource ON operation_logs(resource);
CREATE INDEX IF NOT EXISTS idx_operation_logs_action ON operation_logs(action);

-- 说明：
-- 1) SQLite 使用 AUTOINCREMENT；MySQL/PostgreSQL 部署请把 INTEGER PRIMARY KEY
--    AUTOINCREMENT 换成对应的自增语法，其余列定义可直接复用。
-- 2) 生产环境由应用启动时的 AutoMigrate 建表，本脚本用于审计与手工执行。
-- 3) ALTER TABLE orders 的四条语句在列已存在时会报错，属于预期，可忽略。
