package initializers

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

func LoadEnv() {
	if os.Getenv("RAILWAY_ENVIRONMENT") == "" {
		err := godotenv.Load()
		if err != nil {
			log.Fatal("Error loading .env file")
		} else {
			log.Println(".env file loaded")
		}
	}
}
