package middleware

import (
	"strings"

	"commercial-diving-decompression-control/backend/internal/auth"
	"commercial-diving-decompression-control/backend/internal/util"
	"github.com/gin-gonic/gin"
)

func Auth(service *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			c.Abort()
			util.Fail(c, util.Unauthorized("bearer token is required"))
			return
		}
		claims, err := service.Parse(strings.TrimSpace(parts[1]))
		if err != nil {
			c.Abort()
			util.Fail(c, err)
			return
		}
		c.Set(util.ContextUserID, claims.UserID)
		c.Set(util.ContextUsername, claims.Username)
		c.Set(util.ContextRole, claims.Role)
		c.Next()
	}
}
