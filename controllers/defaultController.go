package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetHome(ctx *gin.Context) {
	message := `Welcome to Amexan API ❤️. Enjoy seamless interaction with this API.

The following are the endpoints for this API:

AUTH
- POST "/signup" - Create user account
- POST "/login" - Access user account
- POST "/verify-email/:activationToken" - Activate user account
- POST "/forgot-password" - Request password reset
- POST "/reset-password/:resetToken" - Reset user password

PRODUCT
- POST "/product" - Create new product
- GET "/product" - Get all products
- POST "/product-specs" - Add product specifications
- POST "/product-images" - Add product images
- GET "/product/{id}" - Get product by ID

ORDER
- POST "/order" - Create a new order
- GET "/order" - Retrieve all orders
- GET "/user/:userId/orders" - Get orders for a specific user
- GET "/order/:orderId" - Get order by ID
- PATCH "/order/:orderId" - Update order status
- DELETE "/order/:orderId" - Delete order by ID`

	ctx.JSON(http.StatusOK, gin.H{
		"message": message,
	})
}
