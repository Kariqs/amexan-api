package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetHome(ctx *gin.Context) {
	welcomeMessage := "Welcome to Amexan API ❤️. Enjoy seamless interection with this API ❤️."
	ctx.JSON(http.StatusOK, welcomeMessage)
}
