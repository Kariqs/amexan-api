package models

import "gorm.io/gorm"

type CartItem struct {
	gorm.Model
	CartID          int
	ProductId       int     `json:"productId"`
	ProductName     string  `json:"productName"`
	ProductPrice    float64 `json:"productPrice"`
	ProductQuantity float64 `product:"productQuantity"`
	ProductImageUrl string  `json:"productImageUrl"`
}

type Cart struct {
	gorm.Model
	UserID int        `json:"userId"`
	Items  []CartItem `gorm:"foreignKey:CartID;constraint:OnDelete:CASCADE"`
}
