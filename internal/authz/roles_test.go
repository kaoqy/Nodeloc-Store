package authz

import "testing"

// TestEffectiveRoleCollapsesLegacy pins the upgrade path: shops that already have
// accounts holding the old role names must keep working after the roles collapsed
// to super_admin / admin.
func TestEffectiveRoleCollapsesLegacy(t *testing.T) {
	cases := map[string]string{
		"super_admin":     "super_admin",
		"admin":           "admin",
		"operator":        "admin",
		"support":         "admin",
		"ops_manager":     "admin",
		"product_manager": "admin",
		"order_manager":   "admin",
		"finance":         "admin",
		"support_lead":    "admin",
		"support_agent":   "admin",
		"ai_admin":        "admin",
		"data_viewer":     "admin",
		"user":            "user",
	}
	for input, want := range cases {
		if got := EffectiveRole(input); got != want {
			t.Errorf("EffectiveRole(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestRolesAreTwo keeps the offered list at exactly two roles.
func TestRolesAreTwo(t *testing.T) {
	if len(Roles) != 2 || Roles[0] != "super_admin" || Roles[1] != "admin" {
		t.Fatalf("Roles = %v, want [super_admin admin]", Roles)
	}
}
