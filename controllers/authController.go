package controllers

import (
	"net/http"
	"os"
	"time"

	"github.com/Kariqs/amexan-api/initializers"
	"github.com/Kariqs/amexan-api/models"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

func Signup(ctx *gin.Context) {
	//extract the data from the request body
	var signUpData models.User
	err := ctx.ShouldBindJSON(&signUpData)
	if err != nil {
		ctx.JSON(400, gin.H{"message": "invalid input"})
		return
	}

	//check if the user already exists
	var existingUser models.User
	user := initializers.DB.Where("email = ? OR username = ?", signUpData.Email, signUpData.Username).Find(&existingUser)

	if user.RowsAffected > 0 {
		ctx.JSON(400, gin.H{"message": "user already exists"})
		return
	}

	//hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(signUpData.Password), 10)
	if err != nil {
		ctx.JSON(500, gin.H{"message": "failed to hash password"})
		return
	}
	signUpData.Password = string(hashedPassword)

	//assign role
	if signUpData.Role == "" {
		signUpData.Role = "user"
	}

	//create the user in the database
	result := initializers.DB.Create(&signUpData)
	if result.Error != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"message": "failed to create user"})
		return
	}
	ctx.JSON(http.StatusCreated, gin.H{"message": "User created successfully"})
}

func Login(ctx *gin.Context) {
	var loginData models.LoginData
	err := ctx.ShouldBindJSON(&loginData)
	if err != nil {
		ctx.JSON(400, gin.H{"message": "invalid input"})
		return
	}

	//check if the user exists
	var user models.User
	result := initializers.DB.Where("email = ? OR username = ?", loginData.Email, loginData.Email).Find(&user)
	if result.RowsAffected == 0 {
		ctx.JSON(400, gin.H{"message": "invalid username or password"})
		return
	}

	//check if the password is correct
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(loginData.Password))
	if err != nil {
		ctx.JSON(400, gin.H{"message": "invalid username or password"})
		return
	}

	//generate a JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  user.ID,
		"email":    user.Email,
		"username": user.Username,
		"role":     user.Role,
		"iat":      time.Now().Unix(),
		"exp":      time.Now().Add(time.Hour * 24 * 30).Unix(),
	})

	jwtSecret := os.Getenv("JWT_SECRET")
	tokenString, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		ctx.JSON(500, gin.H{"message": "failed to generate token"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"token": tokenString})

}
