package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit/domain"
)

func TestSafeCellDefusesFormulaStarts(t *testing.T) {
	for _, value := range []string{"=1+1", "+cmd", "-2", "@SUM(A1)", "\tsheet", "\r"} {
		if got := safeCell(value); got[:1] != "'" {
			t.Errorf("safeCell(%q) = %q, want it prefixed so Excel will not execute it", value, got)
		}
	}
	for _, value := range []string{"", "kaoqy", "order.refund", "192.168.0.1"} {
		if got := safeCell(value); got != value {
			t.Errorf("safeCell(%q) = %q, want it untouched", value, got)
		}
	}
}

// The actor column is the whole reason a log download is readable, so all three
// kinds of entry have to be named: a member, the shop itself, and an account
// that is gone.
func TestAuditActorNamesEveryKindOfEntry(t *testing.T) {
	id := uint(7)
	cases := []struct {
		name  string
		entry domain.AuditLog
		want  string
	}{
		{"member", domain.AuditLog{ActorID: &id, ActorName: "kaoqy"}, "kaoqy"},
		{"system", domain.AuditLog{}, "系统"},
		{"purged account", domain.AuditLog{ActorID: &id}, "#7（账号已删除）"},
	}
	for _, tc := range cases {
		if got := auditActor(tc.entry); got != tc.want {
			t.Errorf("%s: auditActor = %q, want %q", tc.name, got, tc.want)
		}
	}
	if got := auditActor(domain.AuditLog{ActorName: "=HYPERLINK(\"http://evil\")"}); got[:1] != "'" {
		t.Errorf("an actor username starting with = came out as %q", got)
	}
}

// The download reads the same query string the log page writes into its address
// bar, including the Chinese error text for a malformed filter.
func TestParseAuditFiltersReadsThePageQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs/export"+
		"?action=order.&actor=system&search=%20refund%20&since=2026-09-01&until=2026-09-01", nil)

	filter, err := parseAuditFilters(c)
	if err != nil {
		t.Fatal(err)
	}
	if filter.Action != "order." || filter.Search != "refund" {
		t.Fatalf("filter = %+v, want the trimmed action and keyword", filter)
	}
	if !filter.SystemOnly || filter.ActorID != nil {
		t.Fatalf("filter = %+v, want actor=system to mean system-only rows", filter)
	}
	// Picking one day has to cover that day, not stop at its midnight.
	if filter.Since == nil || filter.Before == nil {
		t.Fatalf("filter = %+v, want both bounds set", filter)
	}
	if got := filter.Before.Sub(*filter.Since); got != 24*time.Hour {
		t.Fatalf("one-day window spans %v, want 24h", got)
	}
}

func TestParseAuditFiltersRejectsBadFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for raw, want := range map[string]string{
		"?actor=abc":                         "操作者",
		"?since=yesterday":                   "开始日期",
		"?since=2026-09-05&until=2026-09-01": "不能晚于",
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs/export"+raw, nil)
		_, err := parseAuditFilters(c)
		if err == nil {
			t.Fatalf("%s parsed without an error", raw)
		}
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("%s gave %q, want it to mention %q", raw, err.Error(), want)
		}
	}
}
