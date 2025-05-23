package controllers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/Kariqs/amexan-api/initializers"
	"github.com/Kariqs/amexan-api/models"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Common error response helper
func respondWithError(ctx *gin.Context, statusCode int, message string, err error) {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	ctx.JSON(statusCode, gin.H{
		"message": message,
		"error":   errMsg,
	})
}

// Product handlers
func CreateProduct(ctx *gin.Context) {
	var product models.Product
	if err := ctx.ShouldBindJSON(&product); err != nil {
		respondWithError(ctx, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	if err := initializers.DB.Create(&product).Error; err != nil {
		respondWithError(ctx, http.StatusInternalServerError, "Failed to create product", err)
		return
	}

	ctx.JSON(http.StatusCreated, product)
}

func CreateProductSpecs(ctx *gin.Context) {
	var spec models.ProductSpecs
	if err := ctx.ShouldBindJSON(&spec); err != nil {
		respondWithError(ctx, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// Validate product exists
	var product models.Product
	if err := initializers.DB.First(&product, spec.ProductID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondWithError(ctx, http.StatusNotFound, "Product not found", nil)
		} else {
			respondWithError(ctx, http.StatusInternalServerError, "Failed to validate product", err)
		}
		return
	}

	if err := initializers.DB.Create(&spec).Error; err != nil {
		respondWithError(ctx, http.StatusInternalServerError, "Failed to create product specifications", err)
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{"message": "Product specs added successfully"})
}

// getAWSConfig returns a configured AWS S3 uploader
func getAWSUploader() (*manager.Uploader, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("error loading AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg)
	return manager.NewUploader(client), nil
}

func UploadProductImages(ctx *gin.Context) {
	// Get multipart form
	form, err := ctx.MultipartForm()
	if err != nil {
		respondWithError(ctx, http.StatusBadRequest, "Invalid form data", err)
		return
	}

	files := form.File["images"]
	if len(files) == 0 {
		respondWithError(ctx, http.StatusBadRequest, "No files uploaded", nil)
		return
	}

	// Get and validate productId
	productIdStr := ctx.PostForm("productId")
	if productIdStr == "" {
		respondWithError(ctx, http.StatusBadRequest, "Missing productId", nil)
		return
	}

	productId, err := strconv.Atoi(productIdStr)
	if err != nil {
		respondWithError(ctx, http.StatusBadRequest, "Invalid productId", err)
		return
	}

	// Validate product exists
	var product models.Product
	if err := initializers.DB.First(&product, productId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondWithError(ctx, http.StatusNotFound, "Product not found", nil)
		} else {
			respondWithError(ctx, http.StatusInternalServerError, "Failed to validate product", err)
		}
		return
	}

	// Get AWS uploader
	uploader, err := getAWSUploader()
	if err != nil {
		respondWithError(ctx, http.StatusInternalServerError, "Failed to configure AWS", err)
		return
	}

	var uploadedUrls []string
	var failedUploads []string

	// Upload files and save to database
	for _, file := range files {
		f, openErr := file.Open()
		if openErr != nil {
			log.Printf("Error opening file %s: %v", file.Filename, openErr)
			failedUploads = append(failedUploads, file.Filename)
			continue
		}

		// Generate a unique filename to prevent overwrites
		uniqueFilename := fmt.Sprintf("%d-%s-%s", productId, time.Now().Format("20060102150405"), file.Filename)

		result, uploadErr := uploader.Upload(context.TODO(), &s3.PutObjectInput{
			Bucket:      aws.String("amexan"),
			Key:         aws.String(uniqueFilename),
			Body:        f,
			ACL:         "public-read",
			ContentType: aws.String(file.Header.Get("Content-Type")),
		})
		f.Close() // Close file immediately after use

		if uploadErr != nil {
			log.Printf("Error uploading file %s: %v", file.Filename, uploadErr)
			failedUploads = append(failedUploads, file.Filename)
			continue
		}

		uploadedUrls = append(uploadedUrls, result.Location)

		// Create a ProductImage record
		productImage := models.ProductImage{
			Url:       result.Location,
			ProductID: productId,
		}

		if err := initializers.DB.Create(&productImage).Error; err != nil {
			log.Printf("Error saving image to database: %v", err)
			// We've already uploaded the file, so we'll just log this error
		}
	}

	response := gin.H{
		"message": "Files processed",
		"urls":    uploadedUrls,
	}

	if len(failedUploads) > 0 {
		response["failed"] = failedUploads
	}

	ctx.JSON(http.StatusOK, response)
}

func GetProducts(ctx *gin.Context) {
	var products []models.Product

	// Add pagination
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "4"))
	offset := (page - 1) * limit

	query := initializers.DB.Preload("Images")

	// Add search by name if provided
	if search := ctx.Query("search"); search != "" {
		query = query.Where("name LIKE ?", "%"+search+"%")
	}

	// Execute the query with pagination
	result := query.Limit(limit).Offset(offset).Find(&products)
	if result.Error != nil {
		respondWithError(ctx, http.StatusInternalServerError, "Unable to fetch products", result.Error)
		return
	}

	// Get total count for pagination
	var count int64
	initializers.DB.Model(&models.Product{}).Count(&count)

	ctx.JSON(http.StatusOK, gin.H{
		"products": products,
		"metadata": gin.H{
			"total": count,
			"page":  page,
			"limit": limit,
		},
	})
}

func GetProduct(ctx *gin.Context) {
	productId, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		respondWithError(ctx, http.StatusBadRequest, "Invalid product ID", err)
		return
	}

	var product models.Product
	result := initializers.DB.Preload("Specifications").Preload("Images").First(&product, productId)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			respondWithError(ctx, http.StatusNotFound, "Product not found", nil)
		} else {
			respondWithError(ctx, http.StatusInternalServerError, "Unable to retrieve product", result.Error)
		}
		return
	}

	ctx.JSON(http.StatusOK, product)
}
