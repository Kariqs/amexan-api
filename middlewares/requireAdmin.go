package middlewares

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func RequireAdmin() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userClaims, exists := ctx.Get("user")
		if !exists {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "User not found in context"})
			return
		}

		claims := userClaims.(jwt.MapClaims)
		role, ok := claims["role"].(string)
		if !ok || role != "admin" {
			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "Admin access required"})
			return
		}

		ctx.Next()
	}
}
