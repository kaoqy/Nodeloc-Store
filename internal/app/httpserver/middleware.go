package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/kaoqy/Nodeloc-Store/internal/authz"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
)

const (
	UserIDKey   = "user_id"
	UserRoleKey = "user_role"
	IsAdminKey  = "is_admin"
	// AuditDetailKey is where a handler leaves the one line that makes its own
	// mutation readable afterwards. The URL alone says "somebody moved money";
	// only the handler knows how much and to whom.
	AuditDetailKey = "audit_detail"
)

// JWTMiddleware validates JWT tokens and sets user context.
func JWTMiddleware(cfg *config.JWTConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "请先登录，再打开这个页面。", "code": "unauthenticated"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "登录凭证的格式不对，请重新登录。", "code": "invalid_token"})
			return
		}

		tokenStr := strings.TrimSpace(parts[1])
		claims := &jwt.MapClaims{}

		token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(cfg.Secret), nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "登录状态已失效，请重新登录。", "code": "invalid_token"})
			return
		}

		sub, _ := claims.GetSubject()
		var userID uint
		if sub != "" {
			if id, err := parseUint(sub); err == nil {
				userID = id
			}
		}

		role, _ := (*claims)["role"].(string)
		isAdmin, _ := (*claims)["is_admin"].(bool)

		c.Set(UserIDKey, userID)
		c.Set(UserRoleKey, role)
		c.Set(IsAdminKey, isAdmin)
		c.Next()
	}
}

func parseUint(s string) (uint, error) {
	var result uint
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, jwt.ErrInvalidType
		}
		result = result*10 + uint(c-'0')
	}
	return result, nil
}

// OptionalJWTMiddleware identifies a caller when the request carries a session
// and lets the request through as a guest when it does not. It exists for the
// routes whose answer is public but whose rules are not: a buyer browsing
// without an account may still want to know what a promo code would take off,
// while the 每人限用 that code carries can only be checked against an account.
//
// A token that is present but broken is still refused: silently treating an
// expired session as a guest would show the buyer a discount they cannot get.
func OptionalJWTMiddleware(cfg *config.JWTConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		if authHeader == "" {
			c.Next()
			return
		}
		JWTMiddleware(cfg)(c)
	}
}

// RequirePermission is the back office's gate. It replaces the old
// admin-or-nothing check: the account behind the token is re-read (a JWT only
// carries the role it was signed with), and Casbin then decides whether that
// role may act on this resource. A plain customer therefore gets a clear 403 on
// every admin route, while 运营 and 客服 see only the pages their role grants.
func RequirePermission(reader AccountReader, resource, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := contextUserID(c)
		if userID == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "请先登录，再打开这个页面。", "code": "unauthenticated"})
			return
		}

		role := contextRole(c)
		if reader != nil {
			state, found := reader(c.Request.Context(), userID)
			switch {
			case !found:
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "账号已不存在，请重新登录", "code": "account_missing"})
				return
			case !state.IsActive:
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "账号已被停用，请联系管理员", "code": "account_disabled"})
				return
			}
			role = state.Role
			c.Set(UserRoleKey, role)
			c.Set(IsAdminKey, state.IsAdmin)
		}

		if role == "" || role == "user" || !authz.Can(role, resource, action) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":      "当前账号没有这项后台权限",
				"code":       "permission_denied",
				"permission": resource + ":" + action,
				"your_role":  role,
			})
			return
		}

		c.Next()
	}
}

func contextRole(c *gin.Context) string {
	value, exists := c.Get(UserRoleKey)
	if !exists {
		return ""
	}
	role, _ := value.(string)
	return role
}

// AccountState is an account's current authorization data, read from the store.
type AccountState struct {
	Role     string
	IsAdmin  bool
	IsActive bool
}

// AccountReader reports an account's current state; found is false when the
// account no longer exists.
type AccountReader func(ctx context.Context, userID uint) (state AccountState, found bool)

func contextUserID(c *gin.Context) uint {
	v, exists := c.Get(UserIDKey)
	if !exists {
		return 0
	}
	switch id := v.(type) {
	case uint:
		return id
	case int:
		if id > 0 {
			return uint(id)
		}
	}
	return 0
}

// AuditWriter appends one admin action to the audit trail.
type AuditWriter interface {
	Record(action, target, detail string, actorID uint, ip string)
}

const adminAPIPrefix = "/api/v1/admin/"

// AdminAudit records every successful mutation below the admin API prefix so
// the back office has a trail of who changed what.
func AdminAudit(writer AuditWriter) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if writer == nil || c.Request.Method == http.MethodGet {
			return
		}
		if !strings.HasPrefix(c.Request.URL.Path, adminAPIPrefix) || c.Writer.Status() >= http.StatusBadRequest {
			return
		}
		target := strings.TrimPrefix(c.Request.URL.Path, adminAPIPrefix)
		actor, _ := c.Get(UserIDKey)
		actorID, _ := actor.(uint)
		// A handler that knows more than the URL does (how much moved, which card
		// was deleted) says so through AuditDetailKey; everything else keeps the
		// empty detail it always had.
		detail, _ := c.Get(AuditDetailKey)
		detailText, _ := detail.(string)
		writer.Record(auditAction(c.Request.Method, target), target, detailText, actorID, c.ClientIP())
	}
}

// auditAction turns "orders/NL123/cancel" into "order.cancel".
func auditAction(method, target string) string {
	segments := strings.Split(strings.Trim(target, "/"), "/")
	resource := singularResource(segments[0])
	verb := map[string]string{
		http.MethodPost:   "create",
		http.MethodPut:    "update",
		http.MethodPatch:  "update",
		http.MethodDelete: "delete",
	}[method]
	if verb == "" {
		verb = strings.ToLower(method)
	}
	// A trailing word segment is an explicit action: /orders/:no/refund.
	if last := segments[len(segments)-1]; len(segments) > 1 && !isNumeric(last) {
		verb = last
	} else if len(segments) > 2 {
		// Nested collection item: /products/:id/cards/:cardID targets cards.
		resource = singularResource(segments[len(segments)-2])
	}
	return resource + "." + verb
}

// singularResource turns a URL collection segment into a log-friendly subject.
func singularResource(segment string) string {
	if strings.HasSuffix(segment, "ies") && len(segment) > 3 {
		return segment[:len(segment)-3] + "y"
	}
	return strings.TrimSuffix(segment, "s")
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
