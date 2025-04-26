package initializers

import (
	"log"

	"github.com/Kariqs/amexan-api/models"
)

func SyncDatabase() {
	DB.AutoMigrate(&models.User{}, &models.Product{}, &models.ProductImage{}, &models.ProductSpecs{})
	log.Println("Database synced successfully.")
}
