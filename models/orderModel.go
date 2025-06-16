package models

import "gorm.io/gorm"

type Order struct {
	gorm.Model
	UserID            int         `json:"userId"`
	FirstName         string      `json:"firstName"`
	LastName          string      `json:"lastName"`
	Email             string      `json:"email"`
	Phone             string      `json:"phone"`
	DeliveryLocation  string      `json:"deliveryLocation"`
	Total             float64     `json:"total"`
	Status            string      `json:"status"`
	PesapalTrackingId string      `json:"pesapalTrackingId"`
	PaymentStatus     string      `json:"paymentStatus"`
	OrderItems        []OrderItem `json:"orderItems" gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE"`
}

type OrderItem struct {
	gorm.Model
	OrderID   int     `json:"orderId"`
	ProductId int     `json:"productId"`
	Name      string  `json:"name"`
	Price     float64 `json:"price"`
	Quantity  int     `json:"quantity"`
}
