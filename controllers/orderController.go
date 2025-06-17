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
		log.Println(err)
		sendErrorResponse(ctx, http.StatusBadRequest, "Error parsing request body")
		return
	}

	order := models.Order{
		UserID:           OrderInfo.UserID,
		FirstName:        OrderInfo.FirstName,
		LastName:         OrderInfo.LastName,
		Email:            OrderInfo.Email,
		Phone:            OrderInfo.Phone,
		DeliveryLocation: OrderInfo.DeliveryLocation,
		Total:            OrderInfo.Total,
		Status:           "Pending",
	}

	if err := initializers.DB.Create(&order).Error; err != nil {
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to create order")
		return
	}

	for _, item := range OrderInfo.OrderItems {
		item.OrderID = int(order.ID)
		if err := initializers.DB.Create(&item).Error; err != nil {
			sendErrorResponse(ctx, http.StatusBadRequest, "Failed to create order items")
			return
		}
	}

	token, err := GetPesapalAccessToken()
	if err != nil {
		log.Println(err)
		sendErrorResponse(ctx, http.StatusInternalServerError, "Failed to authenticate with Pesapal")
		return
	}

	pesapalOrder := map[string]any{
		"id":                 fmt.Sprintf("ORDER-%d", order.ID),
		"merchant_reference": fmt.Sprintf("ORDER-%d", order.ID),
		"currency":           "KES",
		"amount":             1, //order.Total
		"description":        "Payment for order #" + fmt.Sprint(order.ID),
		"callback_url":       "https://amexan.store/",
		"notification_id":    os.Getenv("PESAPAL_NOTIFICATION_ID"),
		"billing_address": map[string]any{
			"email_address": order.Email,
			"phone_number":  order.Phone,
			"first_name":    order.FirstName,
			"last_name":     order.LastName,
			"city":          order.DeliveryLocation,
		},
	}

	resp, err := resty.New().R().
		SetHeader("Authorization", "Bearer "+token).
		SetHeader("Content-Type", "application/json").
		SetBody(pesapalOrder).
		Post("https://pay.pesapal.com/v3/api/Transactions/SubmitOrderRequest")

	if err != nil {
		log.Println("Pesapal error:", err)
		sendErrorResponse(ctx, http.StatusInternalServerError, "Failed to submit order to Pesapal")
		return
	}

	var pesapalResp map[string]interface{}
	json.Unmarshal(resp.Body(), &pesapalResp)

	redirectURL, ok := pesapalResp["redirect_url"].(string)
	orderTrackingID, ok2 := pesapalResp["order_tracking_id"].(string)
	if !ok || !ok2 {
		log.Printf("Invalid Pesapal response: %s", resp.Body())
		sendErrorResponse(ctx, http.StatusInternalServerError, "Failed to parse Pesapal response")
		return
	}

	order.PesapalTrackingId = orderTrackingID
	order.PaymentStatus = "PENDING"
	initializers.DB.Save(&order)

	sendJSONResponse(ctx, http.StatusOK, gin.H{
		"message":      "Order created. Redirect user to payment.",
		"redirect_url": redirectURL,
		"order_id":     order.ID,
	})
}

func HandlePesapalIPN(ctx *gin.Context) {
	orderTrackingId := ctx.Query("orderTrackingId")
	if orderTrackingId == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Missing orderTrackingId"})
		return
	}

	token, err := GetPesapalAccessToken()
	if err != nil {
		log.Println("Token error:", err)
		ctx.Status(http.StatusInternalServerError)
		return
	}

	client := resty.New()
	resp, err := client.R().
		SetHeader("Authorization", "Bearer "+token).
		Get("https://pay.pesapal.com/pesapalv3/api/Transactions/GetTransactionStatus?orderTrackingId=" + orderTrackingId)

	if err != nil {
		log.Println("Pesapal status error:", err)
		ctx.Status(http.StatusInternalServerError)
		return
	}

	var result map[string]interface{}
	json.Unmarshal(resp.Body(), &result)

	status, ok := result["payment_status"].(string)
	if !ok {
		log.Println("Invalid status from Pesapal:", resp.String())
		ctx.Status(http.StatusInternalServerError)
		return
	}

	// Update payment status
	if err := initializers.DB.Model(&models.Order{}).
		Where("pesapal_tracking_id = ?", orderTrackingId).
		Update("payment_status", status).Error; err != nil {
		log.Println("DB update error:", err)
		ctx.Status(http.StatusInternalServerError)
		return
	}

	ctx.Status(http.StatusOK)
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
