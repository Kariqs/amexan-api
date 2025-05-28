package models

import "gorm.io/gorm"

type Order struct {
	gorm.Model
	UserID     int
	FirstName  string
	LastName   string
	Email      string
	Phone      string
	OrderItems []OrderItem `gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE"`
}

type OrderItem struct {
	gorm.Model
	OrderID   int
	ProductId int
	Name      string
	Price     float64
	Quantity  int
}
