package models

import (
	"path/filepath"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/platform/database/gormdb"
)

// TestMigrateCreatesMarketAndSupportTables pins the schema this release adds.
// A renamed table silently breaks the module that queries it, so every new
// table is named here rather than discovered at runtime.
func TestMigrateCreatesMarketAndSupportTables(t *testing.T) {
	dir := t.TempDir()
	db, err := gormdb.New(&config.DatabaseConfig{Driver: "sqlite", DSN: filepath.Join(dir, "store.db")})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		// Close before TempDir cleanup, otherwise Windows refuses to remove the file.
		defer sqlDB.Close()
	}
	for _, table := range []string{
		"activities", "activity_rules", "activity_records", "activity_logs", "coupon_records",
		"tickets", "ticket_messages", "ticket_logs", "ticket_attachments", "ticket_ai_sessions",
		"ai_conversations", "ai_messages", "ai_tool_definitions", "ai_tool_permissions", "ai_tool_calls",
		"ai_knowledge", "ai_knowledge_categories", "ai_feedbacks", "ai_configs", "ai_workflow_configs",
		"ai_quick_questions", "customer_service_agents", "customer_service_assignments", "quick_replies",
		"notification_templates", "notification_logs", "system_configs", "operation_logs",
	} {
		if !db.Migrator().HasTable(table) {
			t.Errorf("missing table %s", table)
		}
	}
	for _, column := range []string{"activity_id", "activity_name", "activity_discount", "activity_snapshot"} {
		if !db.Migrator().HasColumn("orders", column) {
			t.Errorf("orders is missing column %s", column)
		}
	}
}
