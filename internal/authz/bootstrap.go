package bootstrap

import (
	"log"

	"github.com/kaoqy/Nodeloc-Store/internal/authz"
	"gorm.io/gorm"
)

// Roles
const (
	RoleSuperAdmin = "super_admin"
	RoleAdmin      = "admin"
	RoleOperator   = "operator"
	RoleSupport    = "support"
	RoleUser       = "user"
)

// Resources
const (
	ResourceDashboard  = "dashboard"
	ResourceProducts   = "products"
	ResourceCards      = "cards"
	ResourceOrders     = "orders"
	ResourceUsers      = "users"
	ResourceCategories = "categories"
	ResourceCoupons    = "coupons"
	ResourceSettings   = "settings"
	ResourceLogs       = "logs"
)

// Actions
const (
	ActionView   = "view"
	ActionManage = "manage"
	ActionDelete = "delete"
)

// InitRBAC initializes RBAC with default roles and permissions
func InitRBAC(db *gorm.DB) error {
	if err := authz.Init(db); err != nil {
		return err
	}

	// Define role-permission mappings
	permissions := map[string]map[string][]string{
		RoleSuperAdmin: {
			"*": {"*"}, // All permissions
		},
		RoleAdmin: {
			ResourceDashboard:  {ActionView},
			ResourceProducts:   {ActionView, ActionManage},
			ResourceCards:      {ActionView, ActionManage},
			ResourceOrders:     {ActionView, ActionManage},
			ResourceUsers:      {ActionView, ActionManage},
			ResourceCategories: {ActionView, ActionManage},
			ResourceCoupons:    {ActionView, ActionManage},
			ResourceSettings:   {ActionView, ActionManage},
			ResourceLogs:       {ActionView},
		},
		RoleOperator: {
			ResourceDashboard:  {ActionView},
			ResourceProducts:   {ActionView, ActionManage},
			ResourceCards:      {ActionView, ActionManage},
			ResourceOrders:     {ActionView, ActionManage},
		},
		RoleSupport: {
			ResourceDashboard: {ActionView},
			ResourceOrders:    {ActionView, ActionManage},
			ResourceUsers:     {ActionView},
		},
	}

	for role, resources := range permissions {
		for resource, actions := range resources {
			for _, action := range actions {
				if resource == "*" {
					// Super admin: add wildcard permission
					authz.Enforcer.AddPolicy(role, "*", "*")
				} else {
					authz.Enforcer.AddPolicy(role, resource, action)
				}
			}
		}
	}

	if err := authz.Enforcer.SavePolicy(); err != nil {
		return err
	}

	log.Println("[bootstrap] RBAC roles and permissions seeded")
	return nil
}
