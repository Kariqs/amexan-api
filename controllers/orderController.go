package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

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
	var orderInfo models.Order
	if err := ctx.ShouldBindJSON(&orderInfo); err != nil {
		log.Printf("JSON binding error: %v", err)
		sendErrorResponse(ctx, http.StatusBadRequest, "Invalid request body")
		return
	}

	tx := initializers.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	order := models.Order{
		UserID:           orderInfo.UserID,
		FirstName:        orderInfo.FirstName,
		LastName:         orderInfo.LastName,
		Email:            orderInfo.Email,
		Phone:            orderInfo.Phone,
		DeliveryLocation: orderInfo.DeliveryLocation,
		Total:            orderInfo.Total,
		Status:           "Pending",
		PaymentStatus:    "PENDING",
	}

	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to create order")
		return
	}

	for _, item := range orderInfo.OrderItems {
		item.OrderID = int(order.ID)
		if err := tx.Create(&item).Error; err != nil {
			tx.Rollback()
			sendErrorResponse(ctx, http.StatusBadRequest, "Failed to create order items")
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		sendErrorResponse(ctx, http.StatusInternalServerError, "Failed to save order")
		return
	}

	token, err := GetPesapalAccessToken()
	if err != nil {
		sendErrorResponse(ctx, http.StatusInternalServerError, "Payment authentication failed")
		return
	}

	notificationID := os.Getenv("PESAPAL_NOTIFICATION_ID")
	if notificationID == "" {
		sendErrorResponse(ctx, http.StatusInternalServerError, "Missing payment configuration")
		return
	}

	pesapalOrder := map[string]any{
		"id":              fmt.Sprintf("ORDER-%d", order.ID),
		"currency":        "KES",
		"amount":          order.Total,
		"description":     fmt.Sprintf("Payment for order #%d", order.ID),
		"callback_url":    "https://amexan.store/payment/callback",
		"notification_id": notificationID,
		"billing_address": map[string]any{
			"email_address": order.Email,
			"phone_number":  order.Phone,
			"country_code":  "KE",
			"first_name":    order.FirstName,
			"last_name":     order.LastName,
			"city":          order.DeliveryLocation,
			"line_1":        order.DeliveryLocation,
		},
	}

	resp, err := resty.New().SetTimeout(30 * time.Second).
		R().
		SetHeaders(map[string]string{
			"Authorization": "Bearer " + token,
			"Accept":        "application/json",
			"Content-Type":  "application/json",
		}).
		SetBody(pesapalOrder).
		Post("https://pay.pesapal.com/v3/api/Transactions/SubmitOrderRequest")

	if err != nil || resp.StatusCode() != 200 {
		log.Printf("Pesapal error: %v, response: %s", err, resp.Body())
		sendErrorResponse(ctx, http.StatusInternalServerError, "Failed to initiate payment")
		return
	}

	var pesapalResp map[string]any
	if err := json.Unmarshal(resp.Body(), &pesapalResp); err != nil {
		sendErrorResponse(ctx, http.StatusInternalServerError, "Invalid response from payment gateway")
		return
	}

	redirectURL, rOK := pesapalResp["redirect_url"].(string)
	orderTrackingID, tOK := pesapalResp["order_tracking_id"].(string)
	if !rOK || !tOK || redirectURL == "" || orderTrackingID == "" {
		sendErrorResponse(ctx, http.StatusInternalServerError, "Incomplete response from payment gateway")
		return
	}

	if err := initializers.DB.Model(&order).Updates(map[string]any{
		"pesapal_tracking_id": orderTrackingID,
		"payment_status":      "PENDING",
		"updated_at":          time.Now(),
	}).Error; err != nil {
		log.Printf("Order %d created, but tracking ID not saved: %s", order.ID, orderTrackingID)
	}

	sendJSONResponse(ctx, http.StatusOK, gin.H{
		"message":           "Order created successfully. Redirect user to payment.",
		"redirect_url":      redirectURL,
		"order_id":          order.ID,
		"order_tracking_id": orderTrackingID,
	})
}

func HandlePesapalIPN(ctx *gin.Context) {
	var trackingId, merchantRef string

	// Read and log raw body
	bodyBytes, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		log.Println("Failed to read IPN body:", err)
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read body"})
		return
	}
	log.Printf("Raw IPN Body: %s\n", string(bodyBytes))
	ctx.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Handle POST body
	if ctx.Request.Method == http.MethodPost {
		type IPNPayload struct {
			OrderTrackingId        string `json:"OrderTrackingId"`
			OrderMerchantReference string `json:"OrderMerchantReference"`
			OrderNotificationType  string `json:"OrderNotificationType"`
		}

		var payload IPNPayload
		if err := json.Unmarshal(bodyBytes, &payload); err != nil {
			log.Println("JSON unmarshal failed:", err)
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
			return
		}
		trackingId = payload.OrderTrackingId
		merchantRef = payload.OrderMerchantReference
	} else {
		// Fallback for GET requests (not recommended for IPNs)
		trackingId = ctx.Query("OrderTrackingId")
		merchantRef = ctx.Query("OrderMerchantReference")
	}

	if trackingId == "" || merchantRef == "" {
		log.Printf("Missing tracking ID or merchant reference: trackingId=%v, merchantRef=%v\n", trackingId, merchantRef)
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Missing parameters"})
		return
	}

	log.Printf("IPN received - Tracking ID: %s | Merchant Ref: %s\n", trackingId, merchantRef)

	// Get access token
	token, err := GetPesapalAccessToken()
	if err != nil {
		log.Println("Failed to get Pesapal access token:", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Authentication with Pesapal failed"})
		return
	}

	// Request payment status
	statusURL := "https://pay.pesapal.com/v3/api/Transactions/GetTransactionStatus?orderTrackingId=" + trackingId
	log.Println("Requesting payment status from Pesapal:", statusURL)

	resp, err := resty.New().R().
		SetHeader("Authorization", "Bearer "+token).
		SetHeader("Accept", "application/json").
		Get(statusURL)

	if err != nil {
		log.Println("Error requesting payment status:", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch transaction status"})
		return
	}

	var statusResp map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &statusResp); err != nil {
		log.Println("Failed to parse Pesapal response:", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid response from Pesapal"})
		return
	}

	log.Printf("Raw response from Pesapal: %s\n", string(resp.Body()))

	if errObj, exists := statusResp["error"]; exists && errObj != nil {
		log.Printf("Pesapal returned an error: %+v\n", errObj)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Error in transaction response"})
		return
	}

	statusDesc := fmt.Sprint(statusResp["payment_status_description"])
	if statusDesc == "" {
		log.Println("Missing payment status in Pesapal response")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Missing payment status"})
		return
	}

	// Update order status in DB
	if err := initializers.DB.Model(&models.Order{}).
		Where("pesapal_tracking_id = ?", trackingId).
		Update("payment_status", statusDesc).Error; err != nil {
		log.Printf("Failed to update order for tracking ID %s: %v\n", trackingId, err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update order status"})
		return
	}

	log.Printf("Payment status updated in DB for tracking ID %s\n", trackingId)

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
