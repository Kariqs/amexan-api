package controllers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/Kariqs/amexan-api/initializers"
	"github.com/Kariqs/amexan-api/models"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func CreateProduct(ctx *gin.Context) {
	var product models.Product
	err := ctx.ShouldBindJSON(&product)
	if err != nil {
		fmt.Println(err)
		log.Fatal("Failed to parse request body")
		return
	}
	result := initializers.DB.Create(&product)
	if result.Error != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "failed to create product"})
		return
	}
	ctx.JSON(http.StatusCreated, product)
}

func CreateProductSpecs(ctx *gin.Context) {
	var spec models.ProductSpecs
	err := ctx.ShouldBindJSON(&spec)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"message": "Invalid request body", "error": err.Error()})
		return
	}
	result := initializers.DB.Create(&spec)
	if result.Error != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"message": "Failed to create product specifications", "error": result.Error.Error()})
		return
	}
	ctx.JSON(http.StatusCreated, gin.H{"message": "Product specs added."})
}

func UploadProductImages(ctx *gin.Context) {
	form, err := ctx.MultipartForm()
	if err != nil {
		log.Printf("error reading multipart form: %v", err)
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid form data"})
		return
	}

	files := form.File["images"]

	if len(files) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "No files uploaded"})
		return
	}

	// Get the productId from form field
	productIdStr := ctx.PostForm("productId")
	if productIdStr == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Missing productId"})
		return
	}

	productId, err := strconv.Atoi(productIdStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid productId"})
		return
	}

	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Printf("error loading AWS config: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to configure AWS"})
		return
	}

	client := s3.NewFromConfig(cfg)
	uploader := manager.NewUploader(client)

	var uploadedUrls []string

	for _, file := range files {
		f, openErr := file.Open()
		if openErr != nil {
			log.Printf("error opening file: %v", openErr)
			continue
		}
		defer f.Close()

		result, uploadErr := uploader.Upload(context.TODO(), &s3.PutObjectInput{
			Bucket:      aws.String("amexan"),
			Key:         aws.String(file.Filename),
			Body:        f,
			ACL:         "public-read",
			ContentType: aws.String(file.Header.Get("Content-Type")),
		})

		if uploadErr != nil {
			log.Printf("error uploading file: %v", uploadErr)
			continue
		}

		uploadedUrls = append(uploadedUrls, result.Location)

		// Create a ProductImage record
		productImage := models.ProductImage{
			Url:       result.Location,
			ProductID: productId,
		}

		if err := initializers.DB.Create(&productImage).Error; err != nil {
			log.Printf("error saving image to database: %v", err)
			continue
		}
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Files uploaded and saved successfully",
		"urls":    uploadedUrls,
	})
}

func GetProducts(ctx *gin.Context) {
	var products []models.Product
	result := initializers.DB.Preload("Images").Find(&products)
	if result.Error != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"message": "unable to fetch products."})
		return
	}
	ctx.JSON(http.StatusOK, products)
}

func GetProduct(ctx *gin.Context) {
	productId, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(400, gin.H{"message": "product Id has some issues"})
		return
	}
	var product models.Product
	result := initializers.DB.Preload("Specifications").Preload("Images").First(&product, productId)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"message": "product not found"})
			return
		}
		ctx.JSON(400, gin.H{"message": "Unable to retrieve product."})
		return
	}
	ctx.JSON(200, product)
}
