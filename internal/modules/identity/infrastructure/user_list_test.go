package infrastructure

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/domain"
)

func newUserListDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.User{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// The user screen still shows one 管理员 filter, but upgraded shops may hold
// historical staff roles. Filtering by staff must include those accounts even
// though they no longer appear as selectable roles.
func TestUserListStaffFilterIncludesLegacyRoles(t *testing.T) {
	repo := NewGormUserRepo(newUserListDB(t))
	ctx := context.Background()
	users := []*domain.User{
		{Username: "buyer", Role: "user", IsActive: true},
		{Username: "legacy-support", Role: "support", IsActive: true},
		{Username: "legacy-operator", Role: "operator", IsActive: true},
		{Username: "owner", Role: "super_admin", IsActive: true},
	}
	for _, user := range users {
		if err := repo.Create(ctx, user); err != nil {
			t.Fatal(err)
		}
	}

	staff, total, err := repo.List(ctx, 20, 0, "", "staff")
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(staff) != 3 {
		t.Fatalf("staff filter returned %d/%d users, want three staff accounts", len(staff), total)
	}
	for _, user := range staff {
		if user.Role == "user" {
			t.Fatalf("staff filter leaked the ordinary user: %+v", user)
		}
	}

	buyers, total, err := repo.List(ctx, 20, 0, "", "user")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(buyers) != 1 || buyers[0].Username != "buyer" {
		t.Fatalf("user filter returned %+v/%d, want only the buyer", buyers, total)
	}
}
