package controllers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Kariqs/amexan-api/initializers"
	"github.com/Kariqs/amexan-api/models"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
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

	previousPage := page - 1
	currentPage := page
	nextPage := page + 1

	var hasNextPage bool
	var hasPreviousPage bool

	totalPages := math.Ceil(float64(count) / float64(limit))
	if currentPage == int(totalPages) {
		hasNextPage = false
	} else {
		hasNextPage = true
	}

	if previousPage == 0 {
		hasPreviousPage = false
	} else {
		hasPreviousPage = true
	}

	ctx.JSON(http.StatusOK, gin.H{
		"products": products,
		"metadata": gin.H{
			"total":        count,
			"currentPage":  currentPage,
			"limit":        limit,
			"hasPrevPage":  hasPreviousPage,
			"hasNextPage":  hasNextPage,
			"previousPage": previousPage,
			"nextPage":     nextPage,
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

func parseS3URL(s3url string) (bucket, key string, err error) {
	parsedURL, err := url.Parse(s3url)
	if err != nil {
		return "", "", err
	}

	hostParts := strings.Split(parsedURL.Host, ".")
	if len(hostParts) >= 3 && hostParts[1] == "s3" {
		bucket = hostParts[0]
		key = strings.TrimPrefix(parsedURL.Path, "/")
	} else {
		pathParts := strings.SplitN(strings.TrimPrefix(parsedURL.Path, "/"), "/", 2)
		if len(pathParts) != 2 {
			return "", "", fmt.Errorf("invalid path in URL: %s", parsedURL.Path)
		}
		bucket = pathParts[0]
		key = pathParts[1]
	}
	return
}

func UpdateProduct(ctx *gin.Context) {
	productId, err := strconv.Atoi(ctx.Param("productId"))
	if err != nil {
		log.Println(err)
		sendErrorResponse(ctx, 400, "Unable to parse peoductId")
		return
	}

	var updateData models.Product
	if err := ctx.ShouldBindJSON(&updateData); err != nil {
		log.Println("Invalid JSON body:", err)
		sendErrorResponse(ctx, 400, "Invalid request body")
		return
	}

	updateData.ID = uint(productId)

	if err := initializers.DB.Model(&models.Product{}).
		Where("id = ?", productId).
		Updates(updateData).Error; err != nil {
		log.Println("Failed to update product:", err)
		sendErrorResponse(ctx, 500, "Failed to update product")
		return
	}

	sendJSONResponse(ctx, 200, gin.H{
		"message": "Product updated successfully",
		"product": updateData,
	})
}

func DeleteProduct(ctx *gin.Context) {
	productId, err := strconv.Atoi(ctx.Param("productId"))
	if err != nil {
		log.Println(err)
		sendErrorResponse(ctx, 400, "Unable to parse productId")
		return
	}

	var productImages []models.ProductImage
	if result := initializers.DB.Where("product_id = ?", productId).Find(&productImages); result.Error != nil {
		log.Println(result.Error)
		sendErrorResponse(ctx, 400, "Could not fetch product images.")
	}

	// Load AWS config and create S3 client
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Println(err)
		sendErrorResponse(ctx, 500, "Failed to load AWS config")
		return
	}
	s3Client := s3.NewFromConfig(cfg)

	// Group by bucket and collect keys
	bucketToKeys := map[string][]types.ObjectIdentifier{}

	for _, img := range productImages {
		if img.Url == "" {
			continue
		}

		bucket, key, err := parseS3URL(img.Url)
		if err != nil {
			log.Printf("Failed to parse S3 URL %s: %v\n", img.Url, err)
			continue
		}
		bucketToKeys[bucket] = append(bucketToKeys[bucket], types.ObjectIdentifier{
			Key: aws.String(key),
		})
	}

	// Delete all keys grouped by bucket
	for bucket, objects := range bucketToKeys {
		if len(objects) == 0 {
			continue
		}
		_, err := s3Client.DeleteObjects(context.TODO(), &s3.DeleteObjectsInput{
			Bucket: aws.String(bucket),
			Delete: &types.Delete{
				Objects: objects,
				Quiet:   aws.Bool(true),
			},
		})
		if err != nil {
			log.Printf("Failed to delete objects from bucket %s: %v\n", bucket, err)
		}
	}
	if result := initializers.DB.Where("product_id = ?", productId).Delete(&models.ProductImage{}); result.Error != nil {
		log.Println(result.Error)
		sendErrorResponse(ctx, 400, "Unable to delete product images.")
		return
	}

	if result := initializers.DB.Where("product_id = ?", productId).Delete(&models.ProductSpecs{}); result.Error != nil {
		log.Println(result.Error)
		sendErrorResponse(ctx, 400, "Unable to delete product specifications.")
		return
	}

	if result := initializers.DB.Delete(&models.Product{}, productId); result.Error != nil {
		log.Println(result.Error)
		sendErrorResponse(ctx, 400, "Unable to delete product.")
		return
	}
	sendJSONResponse(ctx, 200, gin.H{
		"message": "Product was deleted successfully.",
	})
}
