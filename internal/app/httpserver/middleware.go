package middleware

import (
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
)

// JWTMiddleware validates JWT tokens and sets user context.
func JWTMiddleware(cfg *config.JWTConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
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
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
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

// RequirePermission checks if the user has the required permission via Casbin.
func RequirePermission(resource, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get(UserIDKey)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		role, _ := c.Get(UserRoleKey)
		roleStr, _ := role.(string)
		userIDStr, _ := userID.(uint)

		// Super admin always has access
		if roleStr == "super_admin" {
			c.Next()
			return
		}

		if !authz.HasPermission(userIDStr, resource, action) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}

		c.Next()
	}
}

// RequireAdmin checks if user is an admin.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get(UserRoleKey)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		roleStr, _ := role.(string)
		if roleStr != "super_admin" && roleStr != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			return
		}

		c.Next()
	}
}
