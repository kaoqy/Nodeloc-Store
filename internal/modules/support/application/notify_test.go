package application

import "testing"

// TestRenderTemplateSupportsBothSyntaxes keeps the two placeholder styles in
// sync: the seeded templates use {{x}} and older copy used {x}, and a shop may
// have either in its own text.
func TestRenderTemplateSupportsBothSyntaxes(t *testing.T) {
	variables := map[string]string{"ticket_no": "TK1", "status": "已解决"}
	if got := renderTemplate("工单 {{ticket_no}} 变成 {status}", variables); got != "工单 TK1 变成 已解决" {
		t.Fatalf("render = %q", got)
	}
}

// TestNotificationTypeGroups is what the buyer's inbox groups by.
func TestNotificationTypeGroups(t *testing.T) {
	cases := map[string]string{
		"ticket.created":   "ticket",
		"activity.started": "activity",
		"coupon.claimed":   "coupon",
		"order.exception":  "order",
		"other":            "system",
	}
	for key, want := range cases {
		if got := notificationType(key); got != want {
			t.Errorf("notificationType(%q) = %q, want %q", key, got, want)
		}
	}
}
