package main

import (
	"time"

	"github.com/Kariqs/amexan-api/initializers"
	"github.com/Kariqs/amexan-api/routes"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func init() {
	initializers.LoadEnv()
	initializers.ConnectToDB()
	initializers.SyncDatabase()
}

func main() {
	server := gin.Default()

	server.Use(cors.New(cors.Config{
		// Only http:// or https:// domains go here
		AllowOrigins: []string{
			"http://localhost:4200",
			"https://www.freeridgehub.co.ke",
			"https://pay.pesapal.com",
		},

		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "PATCH"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,

		AllowOriginFunc: func(origin string) bool {
			if origin == "" {
				return true
			}
			if origin == "capacitor://localhost" {
				return true
			}
			if origin == "ionic://localhost" {
				return true
			}
			return true
		},
	}))

	routes.DefaultRoutes(server)
	routes.AuthRoutes(server)
	routes.ProductRoutes(server)
	routes.OrderRoutes(server)

	server.Run()
}
