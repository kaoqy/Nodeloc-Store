package authz

import (
	"fmt"
	"log"

	"github.com/casbin/casbin/v2"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"gorm.io/gorm"
)

var Enforcer *casbin.Enforcer

const modelConf = `
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && r.obj == p.obj && r.act == p.act
`

// Init initializes the Casbin enforcer with the database-backed policy store.
func Init(db *gorm.DB) error {
	adapter, err := gormadapter.NewAdapterByDB(db)
	if err != nil {
		return err
	}

	enforcer, err := casbin.NewEnforcer(modelConf, adapter)
	if err != nil {
		return err
	}

	if err := enforcer.LoadPolicy(); err != nil {
		return err
	}

	Enforcer = enforcer
	log.Println("[authz] casbin enforcer initialized")
	return nil
}

// SeedDefaults seeds the default RBAC roles and permissions.
func SeedDefaults() error {
	if Enforcer == nil {
		return nil
	}

	Enforcer.AddPolicy("super_admin", "*", "*")

	adminPerms := [][2]string{
		{"dashboard", "view"}, {"products", "view"}, {"products", "manage"},
		{"cards", "view"}, {"cards", "manage"}, {"orders", "view"}, {"orders", "manage"},
		{"users", "view"}, {"users", "manage"}, {"categories", "view"}, {"categories", "manage"},
		{"coupons", "view"}, {"coupons", "manage"}, {"settings", "view"}, {"settings", "manage"},
		{"logs", "view"},
	}
	for _, p := range adminPerms {
		Enforcer.AddPolicy("admin", p[0], p[1])
	}

	operatorPerms := [][2]string{
		{"dashboard", "view"}, {"products", "view"}, {"products", "manage"},
		{"cards", "view"}, {"cards", "manage"}, {"orders", "view"}, {"orders", "manage"},
	}
	for _, p := range operatorPerms {
		Enforcer.AddPolicy("operator", p[0], p[1])
	}

	supportPerms := [][2]string{
		{"dashboard", "view"}, {"orders", "view"}, {"orders", "manage"}, {"users", "view"},
	}
	for _, p := range supportPerms {
		Enforcer.AddPolicy("support", p[0], p[1])
	}

	if err := Enforcer.SavePolicy(); err != nil {
		return err
	}

	log.Println("[authz] RBAC roles and permissions seeded")
	return nil
}

// AddRoleForUser assigns a role to a user.
func AddRoleForUser(userID, role string) {
	if Enforcer == nil {
		return
	}
	Enforcer.AddRoleForUser(userID, role)
}

// HasPermission checks if a user has permission to perform an action on an object.
func HasPermission(userID uint, obj, act string) bool {
	if Enforcer == nil {
		return false
	}
	ok, _ := Enforcer.Enforce(fmt.Sprintf("%d", userID), obj, act)
	return ok
}

// RolePermissions returns all role-permission mappings.
func RolePermissions() map[string][]string {
	if Enforcer == nil {
		return nil
	}
	policies, _ := Enforcer.GetPolicy()
	result := make(map[string][]string)
	for _, p := range policies {
		if len(p) == 3 {
			role, obj, act := p[0], p[1], p[2]
			result[role] = append(result[role], obj+":"+act)
		}
	}
	return result
}
