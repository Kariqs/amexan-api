package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetHome(ctx *gin.Context) {
	message := `Welcome to Amexan API ❤️. Enjoy seamless interaction with this API.`

	ctx.JSON(http.StatusOK, gin.H{
		"message": message,
	})
}
