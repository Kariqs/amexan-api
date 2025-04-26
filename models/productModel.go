package models

import (
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ProductSpecs struct {
	gorm.Model
	Label     string `json:"label" binding:"required"`
	Value     string `json:"value" binding:"required"`
	ProductID int    `json:"productId" binding:"required"`
}

type ProductImage struct {
	gorm.Model
	Url       string `json:"url" binding:"required"`
	ProductID int    `json:"productId" binding:"required"`
}

type Product struct {
	gorm.Model
	Brand          string         `json:"brand" binding:"required"`
	Name           string         `json:"name" binding:"required"`
	Description    string         `json:"description" binding:"required"`
	Price          int            `json:"price" binding:"required"`
	Category       string         `json:"category" binding:"required"`
	Colors         datatypes.JSON `json:"colors"`
	Specifications []ProductSpecs `gorm:"foreignKey:ProductID;constraint:OnDelete:CASCADE"`
	Images         []ProductImage `gorm:"foreignKey:ProductID;constraint:OnDelete:CASCADE"`
}
