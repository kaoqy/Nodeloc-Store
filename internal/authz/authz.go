package authz

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/casbin/casbin/v3"
	"github.com/casbin/casbin/v3/model"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"gorm.io/gorm"
)

var (
	Enforcer *casbin.Enforcer
	// policyDB keeps the handle Init was given: the seed marker lives in
	// app_settings, which is not something the enforcer itself reads.
	policyDB *gorm.DB
)

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

	m, err := model.NewModelFromString(modelConf)
	if err != nil {
		return err
	}

	enforcer, err := casbin.NewEnforcer(m, adapter)
	if err != nil {
		return err
	}

	if err := enforcer.LoadPolicy(); err != nil {
		return err
	}

	Enforcer = enforcer
	policyDB = db
	log.Println("[authz] casbin enforcer initialized")
	return nil
}

// SeedDefaults seeds the default RBAC roles and permissions.
//
// Seeding is versioned and a role is only seeded while it has no policies at
// all. Re-applying the whole list on every boot used to resurrect grants the
// shop owner had deliberately removed, because the container is rebuilt after
// each settings save; a version step only adds what a newer release introduced
// and never rewrites what is already there.
func SeedDefaults() error {
	if Enforcer == nil {
		return nil
	}

	seed := map[string][][2]string{
		// 管理员：覆盖日常经营与客服。工单必须能看能回，这是店主每天都要做的事。
		"admin": {
			{"stats", "view"}, {"stats", "export"},
			{"products", "view"}, {"products", "manage"},
			{"cards", "view"}, {"cards", "manage"},
			{"orders", "view"}, {"orders", "manage"},
			{"categories", "view"}, {"categories", "manage"},
			{"coupons", "view"}, {"coupons", "manage"},
			{"activities", "view"}, {"activities", "manage"},
			{"tickets", "view"}, {"tickets", "manage"}, {"tickets", "assign"},
			{"agents", "view"}, {"agents", "manage"},
			{"quick_replies", "view"}, {"quick_replies", "manage"},
			{"notification_templates", "view"}, {"notification_templates", "manage"},
			{"config_center", "view"}, {"config_center", "manage"},
			{"ticket_config", "view"}, {"ticket_config", "manage"},
			{"users", "view"}, {"users", "manage"},
			{"notifications", "view"}, {"notifications", "manage"},
			{"plugins", "view"}, {"plugins", "manage"},
			{"settings", "view"}, {"settings", "manage"},
			{"roles", "view"}, {"logs", "view"}, {"operation_logs", "view"},
			{"finance", "view"},
		},
	}

	stored := readSeedVersion()
	changed := false
	if stored < 1 {
		if !hasPolicies("super_admin") {
			Enforcer.AddPolicy("super_admin", "*", "*")
			changed = true
		}
		for role, permissions := range seed {
			if hasPolicies(role) {
				continue
			}
			for _, permission := range permissions {
				Enforcer.AddPolicy(role, permission[0], permission[1])
				changed = true
			}
		}
	}
	if stored < 2 {
		// The role editor went live with 细分权限, and 管理员 is the role a shop
		// owner uses for day-to-day work. Without roles:manage an existing store
		// could not add its first staff account at all.
		if addMissing("admin", "roles", "manage") {
			changed = true
		}
	}
	if stored < 3 {
		if migrateLegacyGrants(seed) {
			changed = true
		}
	}
	if stored < 4 {
		// 插件管理 arrived as its own resource. An existing 管理员/运营 role was
		// seeded before it existed, so the grant has to be added explicitly;
		// otherwise the screen would be there but every request would 403.
		if addMissing("admin", "plugins", "view") {
			changed = true
		}
		if addMissing("admin", "plugins", "manage") {
			changed = true
		}
		if addMissing("operator", "plugins", "view") {
			changed = true
		}
		if addMissing("operator", "plugins", "manage") {
			changed = true
		}
	}
	if stored < 7 {
		// 角色收敛到 super_admin / admin 之后，补齐管理员日常所需的全部权限。
		// 历史角色（operator / support 等）如果已经有了策略，也一并补齐，
		// 因为它们在权限判定里等同 admin，账号不会因为升级而失去入口。
		targets := []string{"admin"}
		for _, legacy := range LegacyRoles {
			if hasPolicies(legacy) {
				targets = append(targets, legacy)
			}
		}
		for _, role := range targets {
			for _, permission := range seed["admin"] {
				if addMissing(role, permission[0], permission[1]) {
					changed = true
				}
			}
		}
	}
	if !changed {
		return nil
	}

	if err := Enforcer.SavePolicy(); err != nil {
		return err
	}
	if err := writeSeedVersion(seedVersion); err != nil {
		log.Printf("[authz] record seed version: %v", err)
	}
	log.Println("[authz] RBAC roles and permissions seeded")
	return nil
}

// addMissing grants one permission to a role unless it already holds it.
func addMissing(role, obj, act string) bool {
	if Enforcer == nil {
		return false
	}
	ok, err := Enforcer.Enforce(role, obj, act)
	if err != nil || ok {
		return false
	}
	// A role with no policies at all keeps whatever the version step decided,
	// so an untouched role is not half-seeded by a single grant.
	if !hasPolicies(role) {
		return false
	}
	Enforcer.AddPolicy(role, obj, act)
	return true
}

// migrateLegacyGrants brings a policy set written before the permission catalog
// settled up to this release. 概览 was seeded as dashboard:view, but every route
// and the whole back office ask for stats:view, so a store that predates the
// rename would find its own dashboard returning 403 — and 管理员 had no
// roles:view either, which is the one screen that could fix it.
//
// A grant naming a resource this release does not know is dropped rather than
// kept: the matcher compares literals, so a stale row can never match a request
// and the role editor would show a permission that gates nothing.
func migrateLegacyGrants(seed map[string][][2]string) bool {
	if Enforcer == nil {
		return false
	}
	policies, err := Enforcer.GetPolicy()
	if err != nil {
		log.Printf("[authz] read policies for migration: %v", err)
		return false
	}

	changed := false
	var stale [][]string
	for _, policy := range policies {
		if len(policy) != 3 || policy[0] == "super_admin" {
			continue
		}
		role, obj, act := policy[0], policy[1], policy[2]
		if grantable(obj, act) {
			continue
		}
		if obj == legacyStatsResource && act == "view" && addMissing(role, "stats", "view") {
			// Carry the renamed grant over before the old row goes away, so the
			// role is never left without a dashboard at any point.
			changed = true
		}
		stale = append(stale, policy)
	}
	for _, policy := range stale {
		if _, err := Enforcer.RemoveFilteredPolicy(0, policy...); err != nil {
			log.Printf("[authz] drop legacy policy %v: %v", policy, err)
			continue
		}
		changed = true
	}

	// The seeded lists grew alongside the editor; a store has no way to hand out
	// grants its build never knew about.
	for role, permissions := range seed {
		for _, permission := range permissions {
			if addMissing(role, permission[0], permission[1]) {
				changed = true
			}
		}
	}
	return changed
}

const legacyStatsResource = "dashboard"

const (
	// seedVersion is the shape of the seeded policy set this build expects.
	seedVersion        = 7
	seedVersionKey     = "authz_seed_version"
	settingsValueTable = "app_settings"
)

// readSeedVersion is 0 for a store that has never been seeded, including one
// whose casbin_rule rows predate versioning — those get the version-1 rules
// left alone and only the later steps applied.
func readSeedVersion() int {
	if policyDB == nil {
		return seedVersion
	}
	var stored []string
	err := policyDB.Table(settingsValueTable).
		Where("key = ?", seedVersionKey).
		Pluck("value", &stored).Error
	if err != nil || len(stored) == 0 {
		return 0
	}
	version, err := strconv.Atoi(strings.TrimSpace(stored[0]))
	if err != nil {
		return 0
	}
	return version
}

func writeSeedVersion(version int) error {
	if policyDB == nil {
		return nil
	}
	return policyDB.Exec(
		"INSERT INTO "+settingsValueTable+" (key, value, created_at, updated_at) VALUES (?, ?, ?, ?)"+
			" ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at",
		seedVersionKey, strconv.Itoa(version), time.Now().UTC(), time.Now().UTC()).Error
}

// PermissionGroup is one row of the role editor: a back-office area and the
// actions it distinguishes.
type PermissionGroup struct {
	Resource string   `json:"resource"`
	Label    string   `json:"label"`
	Actions  []string `json:"actions"`
}

// PermissionCatalog is everything that can be granted. The role editor renders
// this instead of letting anyone type policy strings by hand.
func PermissionCatalog() []PermissionGroup {
	return []PermissionGroup{
		{Resource: "stats", Label: "概览与统计", Actions: []string{"view", "export"}},
		{Resource: "finance", Label: "财务数据", Actions: []string{"view", "export"}},
		{Resource: "activities", Label: "活动营销", Actions: []string{"view", "manage"}},
		{Resource: "tickets", Label: "工单中心", Actions: []string{"view", "manage", "assign"}},
		{Resource: "ticket_config", Label: "工单配置", Actions: []string{"view", "manage"}},
		{Resource: "agents", Label: "客服人员", Actions: []string{"view", "manage"}},
		{Resource: "quick_replies", Label: "快捷回复", Actions: []string{"view", "manage"}},
		{Resource: "notification_templates", Label: "通知模板", Actions: []string{"view", "manage"}},
		{Resource: "config_center", Label: "配置中心", Actions: []string{"view", "manage"}},
		{Resource: "operation_logs", Label: "操作日志", Actions: []string{"view", "export"}},
		{Resource: "products", Label: "商品", Actions: []string{"view", "manage"}},
		{Resource: "categories", Label: "分类", Actions: []string{"view", "manage"}},
		{Resource: "cards", Label: "卡密", Actions: []string{"view", "manage"}},
		{Resource: "coupons", Label: "优惠码", Actions: []string{"view", "manage"}},
		{Resource: "orders", Label: "订单", Actions: []string{"view", "manage"}},
		{Resource: "users", Label: "用户", Actions: []string{"view", "manage"}},
		{Resource: "notifications", Label: "通知", Actions: []string{"view", "manage"}},
		{Resource: "settings", Label: "设置", Actions: []string{"view", "manage"}},
		{Resource: "roles", Label: "角色权限", Actions: []string{"view", "manage"}},
		{Resource: "logs", Label: "审计日志", Actions: []string{"view"}},
		{Resource: "plugins", Label: "插件", Actions: []string{"view", "manage"}},
	}
}

// hasPolicies reports whether a role already carries grants, which decides
// whether seeding should leave it alone.
func hasPolicies(role string) bool {
	policies, err := Enforcer.GetFilteredPolicy(0, role)
	if err != nil {
		log.Printf("[authz] read policies for %s: %v", role, err)
		return true
	}
	return len(policies) > 0
}

// Roles are the back-office roles the application understands. They are a
// fixed set because the storefront's navigation and the seeded policy lists
// both assume exactly these names.
// Roles 是后台提供的角色。只保留两个：
//
//	super_admin —— 拥有全部权限，负责配置与人员；
//	admin       —— 日常经营与客服，能回复工单、处理订单与商品。
//
// LegacyRoles 是历史版本用过的名字。它们不再作为可选项出现，但仍然被识别，
// 并在权限判定时按 admin 处理：已经有这些角色的账号不会因为升级而失去访问权。
var Roles = []string{"super_admin", "admin"}

var LegacyRoles = []string{
	"operator", "support",
	"ops_manager", "product_manager", "order_manager", "finance",
	"support_lead", "support_agent", "ai_admin", "data_viewer",
}

// EffectiveRole 把历史角色名归一化成当前的两个角色之一。
// 权限判定与界面展示都走这里，升级后旧账号仍然是原样可用。
func EffectiveRole(role string) string {
	role = strings.TrimSpace(role)
	if role == "super_admin" {
		return "super_admin"
	}
	for _, legacy := range LegacyRoles {
		if role == legacy {
			return "admin"
		}
	}
	return role
}

// Can reports whether a role may perform act on obj. Policies are keyed by
// role, not by user id, which is what makes a permission edit apply to everyone
// holding that role at once.
func Can(role, obj, act string) bool {
	if Enforcer == nil || role == "" {
		return false
	}
	role = EffectiveRole(role)
	if role == "super_admin" {
		return true
	}
	ok, err := Enforcer.Enforce(role, obj, act)
	if err != nil {
		log.Printf("[authz] enforce %s %s %s: %v", role, obj, act, err)
		return false
	}
	return ok
}

// PermissionsOf returns the "object:action" list granted to a role.
func PermissionsOf(role string) []string {
	if Enforcer == nil {
		return nil
	}
	return RolePermissions()[role]
}

// SetRolePermissions replaces one role's grants. super_admin is refused: it
// holds the wildcard policy, and editing it is how an admin locks themselves
// out of the shop with no way back in except the database.
func SetRolePermissions(role string, permissions []string) error {
	if Enforcer == nil {
		return ErrEnforcerNotReady
	}
	if role == "super_admin" {
		return ErrSuperAdminLocked
	}
	if !knownRole(role) {
		return fmt.Errorf("%w: %q", ErrUnknownRole, role)
	}
	// Validated up front: the old policies are already gone by the time the
	// writes start, so a bad entry found halfway would leave the role empty.
	grants := make([][2]string, 0, len(permissions))
	for _, permission := range permissions {
		obj, act, ok := splitPermission(permission)
		if !ok {
			return fmt.Errorf("%w: %q", ErrMalformedPermission, permission)
		}
		if !grantable(obj, act) {
			return fmt.Errorf("%w: %q", ErrUnknownPermission, permission)
		}
		grants = append(grants, [2]string{obj, act})
	}
	if _, err := Enforcer.RemoveFilteredPolicy(0, role); err != nil {
		return err
	}
	for _, grant := range grants {
		if _, err := Enforcer.AddPolicy(role, grant[0], grant[1]); err != nil {
			return err
		}
	}
	return Enforcer.SavePolicy()
}

// grantable keeps the policy table to the vocabulary the routes actually check.
// A typo in the role editor would otherwise look like a saved grant that gates
// nothing.
func grantable(resource, action string) bool {
	for _, group := range PermissionCatalog() {
		if group.Resource != resource {
			continue
		}
		for _, candidate := range group.Actions {
			if candidate == action {
				return true
			}
		}
	}
	return false
}

func knownRole(role string) bool {
	for _, candidate := range Roles {
		if candidate == role {
			return true
		}
	}
	return false
}

func splitPermission(value string) (string, string, bool) {
	obj, act, found := strings.Cut(strings.TrimSpace(value), ":")
	if !found || obj == "" || act == "" {
		return "", "", false
	}
	return obj, act, true
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
