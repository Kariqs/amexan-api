package main

import (
	"github.com/Kariqs/amexan-api/initializers"
	"github.com/Kariqs/amexan-api/routes"
	"github.com/gin-gonic/gin"
)

func init() {
	initializers.LoadEnv()
	initializers.ConnectToDB()
	initializers.SyncDatabase()
}

func main() {
	server := gin.Default()
	routes.DefaultRoutes(server)
	routes.AuthRoutes(server)
	server.Run()
}
