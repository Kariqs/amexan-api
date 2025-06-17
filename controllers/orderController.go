package controllers

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"

	"github.com/Kariqs/amexan-api/initializers"
	"github.com/Kariqs/amexan-api/models"
	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"
)

func GetPesapalAccessToken() (string, error) {
	consumerKey := os.Getenv("PESAPAL_CONSUMER_KEY")
	consumerSecret := os.Getenv("PESAPAL_CONSUMER_SECRET")

	if consumerKey == "" || consumerSecret == "" {
		return "", fmt.Errorf("pesapal consumer credentials are not set")
	}

	requestBody := map[string]string{
		"consumer_key":    consumerKey,
		"consumer_secret": consumerSecret,
	}

	client := resty.New()
	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json").
		SetBody(requestBody).
		Post("https://pay.pesapal.com/v3/api/Auth/RequestToken")

	if err != nil {
		return "", err
	}

	if resp.StatusCode() != 200 {
		return "", fmt.Errorf("pesapal token request failed with status %d: %s", resp.StatusCode(), string(resp.Body()))
	}

	var response map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &response); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	token, ok := response["token"].(string)
	if !ok || token == "" {
		return "", fmt.Errorf("token not found in response: %v", response)
	}

	return token, nil
}

func CreateOrder(ctx *gin.Context) {
	var OrderInfo models.Order
	if err := ctx.ShouldBindJSON(&OrderInfo); err != nil {
		log.Printf("❌ JSON binding error: %v", err)
		sendErrorResponse(ctx, http.StatusBadRequest, "Error parsing request body")
		return
	}

	// Start database transaction
	tx := initializers.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	order := models.Order{
		UserID:           OrderInfo.UserID,
		FirstName:        OrderInfo.FirstName,
		LastName:         OrderInfo.LastName,
		Email:            OrderInfo.Email,
		Phone:            OrderInfo.Phone,
		DeliveryLocation: OrderInfo.DeliveryLocation,
		Total:            OrderInfo.Total,
		Status:           "Pending",
		PaymentStatus:    "PENDING", // Initialize payment status
	}

	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		log.Printf("❌ Failed to create order: %v", err)
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to create order")
		return
	}

	// Create order items
	for _, item := range OrderInfo.OrderItems {
		item.OrderID = int(order.ID)
		if err := tx.Create(&item).Error; err != nil {
			tx.Rollback()
			log.Printf("❌ Failed to create order item: %v", err)
			sendErrorResponse(ctx, http.StatusBadRequest, "Failed to create order items")
			return
		}
	}

	// Commit the transaction before calling Pesapal
	if err := tx.Commit().Error; err != nil {
		log.Printf("❌ Failed to commit transaction: %v", err)
		sendErrorResponse(ctx, http.StatusInternalServerError, "Failed to save order")
		return
	}

	// Get Pesapal token
	token, err := GetPesapalAccessToken()
	if err != nil {
		log.Printf("❌ Token error: %v", err)
		sendErrorResponse(ctx, http.StatusInternalServerError, "Failed to authenticate with Pesapal")
		return
	}

	// Validate required environment variables
	notificationID := os.Getenv("PESAPAL_NOTIFICATION_ID")
	if notificationID == "" {
		log.Println("❌ PESAPAL_NOTIFICATION_ID environment variable not set")
		sendErrorResponse(ctx, http.StatusInternalServerError, "Payment configuration error")
		return
	}

	// Create Pesapal order payload
	pesapalOrder := map[string]any{
		"id":                 fmt.Sprintf("ORDER-%d", order.ID),
		"currency":           "KES",
		"amount":             order.Total, // ✅ Use actual amount, not hardcoded 1
		"description":        "Payment for order #" + fmt.Sprint(order.ID),
		"callback_url":       "https://amexan.store/payment/callback", // ✅ Use specific callback URL
		"notification_id":    notificationID,
		"billing_address": map[string]any{
			"email_address": order.Email,
			"phone_number":  order.Phone,
			"country_code":  "KE", // ✅ Add country code
			"first_name":    order.FirstName,
			"last_name":     order.LastName,
			"city":          order.DeliveryLocation,
			"line_1":        order.DeliveryLocation, // ✅ Add address line
		},
	}

	log.Printf("📤 Submitting order to Pesapal: %+v", pesapalOrder)

	// Submit to Pesapal
	client := resty.New().SetTimeout(30 * time.Second) // ✅ Set timeout
	resp, err := client.R().
		SetHeader("Authorization", "Bearer "+token).
		SetHeader("Accept", "application/json").
		SetHeader("Content-Type", "application/json").
		SetBody(pesapalOrder).
		Post("https://pay.pesapal.com/v3/api/Transactions/SubmitOrderRequest")

	if err != nil {
		log.Printf("❌ Pesapal request error: %v", err)
		sendErrorResponse(ctx, http.StatusInternalServerError, "Failed to submit order to Pesapal")
		return
	}

	// ✅ Check HTTP status code
	if resp.StatusCode() != 200 {
		log.Printf("❌ Pesapal returned status %d: %s", resp.StatusCode(), string(resp.Body()))
		sendErrorResponse(ctx, http.StatusInternalServerError, "Payment gateway error")
		return
	}

	// ✅ Log the full response for debugging
	log.Printf("📋 Pesapal Response Status: %d", resp.StatusCode())
	log.Printf("📋 Pesapal Response Body: %s", string(resp.Body()))

	var pesapalResp map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &pesapalResp); err != nil {
		log.Printf("❌ JSON unmarshal error: %v", err)
		sendErrorResponse(ctx, http.StatusInternalServerError, "Invalid response from payment gateway")
		return
	}

	// ✅ Check for errors in Pesapal response
	if errorObj, exists := pesapalResp["error"]; exists && errorObj != nil {
		log.Printf("❌ Pesapal error response: %+v", errorObj)
		sendErrorResponse(ctx, http.StatusInternalServerError, "Payment gateway error")
		return
	}

	// ✅ Check for status field
	if status, exists := pesapalResp["status"]; exists {
		if statusCode, ok := status.(float64); ok && statusCode != 200 {
			log.Printf("❌ Pesapal status error: %v", pesapalResp)
			sendErrorResponse(ctx, http.StatusInternalServerError, "Payment gateway error")
			return
		}
	}

	// ✅ Extract redirect URL and tracking ID with better error handling
	redirectURL, redirectOK := pesapalResp["redirect_url"].(string)
	orderTrackingID, trackingOK := pesapalResp["order_tracking_id"].(string)

	if !redirectOK || !trackingOK || redirectURL == "" || orderTrackingID == "" {
		log.Printf("❌ Missing required fields in Pesapal response:")
		log.Printf("   redirect_url: %v (type: %T)", pesapalResp["redirect_url"], pesapalResp["redirect_url"])
		log.Printf("   order_tracking_id: %v (type: %T)", pesapalResp["order_tracking_id"], pesapalResp["order_tracking_id"])
		log.Printf("   Full response: %+v", pesapalResp)
		sendErrorResponse(ctx, http.StatusInternalServerError, "Invalid payment gateway response")
		return
	}

	// ✅ Update order with tracking ID
	if err := initializers.DB.Model(&order).Updates(map[string]interface{}{
		"pesapal_tracking_id": orderTrackingID,
		"payment_status":      "PENDING",
		"updated_at":          time.Now(),
	}).Error; err != nil {
		log.Printf("❌ Failed to update order with tracking ID: %v", err)
		// Don't fail the request since Pesapal already has the order
		// But log it for manual intervention
		log.Printf("⚠️  CRITICAL: Order %d created but tracking ID not saved: %s", order.ID, orderTrackingID)
	}

	log.Printf("✅ Order created successfully:")
	log.Printf("   Order ID: %d", order.ID)
	log.Printf("   Tracking ID: %s", orderTrackingID)
	log.Printf("   Redirect URL: %s", redirectURL)

	sendJSONResponse(ctx, http.StatusOK, gin.H{
		"message":           "Order created successfully. Redirect user to payment.",
		"redirect_url":      redirectURL,
		"order_id":          order.ID,
		"order_tracking_id": orderTrackingID,
	})
}

func HandlePesapalIPN(ctx *gin.Context) {
	var trackingId, merchantRef string

	// Support GET or POST
	if ctx.Request.Method == http.MethodPost {
		var payload map[string]any
		if err := ctx.BindJSON(&payload); err != nil {
			log.Println("Invalid IPN body:", err)
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
			return
		}

		trackingId = fmt.Sprint(payload["orderTrackingId"])
		merchantRef = fmt.Sprint(payload["orderMerchantReference"])
	} else {
		trackingId = ctx.Query("orderTrackingId")
		merchantRef = ctx.Query("orderMerchantReference")
	}

	if trackingId == "" || merchantRef == "" {
		log.Println("Missing trackingId or merchantReference")
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Missing parameters"})
		return
	}

	// Get Pesapal Access Token
	token, err := GetPesapalAccessToken()
	if err != nil {
		log.Println("Token error:", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Auth failed"})
		return
	}

	// Check transaction status
	client := resty.New()
	resp, err := client.R().
		SetHeader("Authorization", "Bearer "+token).
		SetHeader("Accept", "application/json").
		SetHeader("Content-Type", "application/json").
		Get("https://pay.pesapal.com/v3/api/Transactions/GetTransactionStatus?orderTrackingId=" + trackingId)

	if err != nil {
		log.Println("Status fetch error:", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Status check failed"})
		return
	}

	var result map[string]interface{}
	json.Unmarshal(resp.Body(), &result)
	log.Printf("Payment Status Response: %+v\n", result)

	statusDesc := fmt.Sprint(result["payment_status_description"])

	// Update DB
	if err := initializers.DB.Model(&models.Order{}).
		Where("pesapal_tracking_id = ?", trackingId).
		Update("payment_status", statusDesc).Error; err != nil {
		log.Println("DB update error:", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update DB"})
		return
	}

	// ✅ Respond to Pesapal
	ctx.JSON(http.StatusOK, gin.H{
		"orderNotificationType":  "IPNCHANGE",
		"orderTrackingId":        trackingId,
		"orderMerchantReference": merchantRef,
		"status":                 200,
	})
}


func GetOrders(ctx *gin.Context) {
	var orders []models.Order

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "15"))
	offset := (page - 1) * limit

	sortOrder := ctx.DefaultQuery("sort", "desc")
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	query := initializers.DB.Preload("OrderItems")

	if search := ctx.Query("search"); search != "" {
		query = query.Where("ID LIKE ?", "%"+search+"%")
	}

	query = query.Order("created_at " + sortOrder)

	result := query.Limit(limit).Offset(offset).Find(&orders)
	if result.Error != nil {
		respondWithError(ctx, http.StatusInternalServerError, "Unable to fetch orders", result.Error)
		return
	}

	var count int64
	countQuery := initializers.DB.Model(&models.Order{})
	if search := ctx.Query("search"); search != "" {
		countQuery = countQuery.Where("id LIKE ?", "%"+search+"%")
	}
	countQuery.Count(&count)

	previousPage := page - 1
	nextPage := page + 1
	totalPages := math.Ceil(float64(count) / float64(limit))

	ctx.JSON(http.StatusOK, gin.H{
		"orders": orders,
		"metadata": gin.H{
			"total":        count,
			"currentPage":  page,
			"limit":        limit,
			"hasPrevPage":  previousPage > 0,
			"hasNextPage":  int(totalPages) > page,
			"previousPage": previousPage,
			"nextPage":     nextPage,
		},
	})
}

func GetOderByCustomerId(ctx *gin.Context) {
	userId, err := strconv.Atoi(ctx.Param("userId"))
	if err != nil {
		log.Println(err)
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to parse userId")
		return
	}

	sortOrder := ctx.DefaultQuery("sort", "desc")
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	query := initializers.DB.Preload("OrderItems").Where("user_id = ?", userId)

	if search := ctx.Query("search"); search != "" {
		query = query.Where("id LIKE ?", "%"+search+"%")
	}

	var orders []models.Order
	if result := query.Order("created_at " + sortOrder).Find(&orders); result.Error != nil {
		log.Println(result.Error)
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to fetch orders.")
		return
	}

	sendJSONResponse(ctx, http.StatusOK, gin.H{
		"orders": orders,
	})
}

func GetOderById(ctx *gin.Context) {
	orderId, err := strconv.Atoi(ctx.Param("orderId"))
	if err != nil {
		log.Println(err)
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to parse orderId")
		return
	}

	var order models.Order
	if result := initializers.DB.Preload("OrderItems").Where("id = ?", orderId).Find(&order); result.Error != nil {
		log.Println(result.Error)
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to fetch order.")
		return
	}

	sendJSONResponse(ctx, http.StatusOK, gin.H{
		"order": order,
	})
}

func UpdateOrderStatus(ctx *gin.Context) {
	var orderStatusData struct {
		Status string `json:"status"`
	}
	err := ctx.ShouldBindJSON(&orderStatusData)
	if err != nil {
		log.Println(err)
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to parse request body")
		return
	}

	orderId, err := strconv.Atoi(ctx.Param("orderId"))
	if err != nil {
		log.Println(err)
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to parse orderId")
		return
	}
	if result := initializers.DB.Model(&models.Order{}).Where("id = ?", orderId).Update("status", orderStatusData.Status); result.Error != nil {
		log.Println(result.Error)
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to update order status")
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Order status updated successfully.",
	})
}

func DeleteOrder(ctx *gin.Context) {
	orderId, err := strconv.Atoi(ctx.Param("orderId"))
	if err != nil {
		log.Println(err)
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to parse order id.")
		return
	}

	if result := initializers.DB.Delete(&models.Order{}, orderId); result.Error != nil {
		log.Println(result.Error)
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to delete order.")
		return
	}

	sendJSONResponse(ctx, http.StatusOK, gin.H{"message": "Order deleted successfully."})
}

func GetUndeliveredOrders(ctx *gin.Context) {
	var count int64

	result := initializers.DB.
		Model(&models.Order{}).
		Where("status != ?", "Completed").
		Count(&count)

	if result.Error != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count undelivered orders"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"undeliveredOrderCount": count})
}
