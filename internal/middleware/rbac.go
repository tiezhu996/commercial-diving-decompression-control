package middleware

import (
	"commercial-diving-decompression-control/backend/internal/util"
	"github.com/gin-gonic/gin"
)

func RBAC(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(roles))
	for _, role := range roles {
		allowed[role] = true
	}
	return func(c *gin.Context) {
		role := util.Role(c)
		if role == "" || !allowed[role] {
			c.Abort()
			util.Fail(c, util.Forbidden("current role cannot perform this operation"))
			return
		}
		c.Next()
	}
}
