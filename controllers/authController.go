package controllers

import (
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/Kariqs/amexan-api/initializers"
	"github.com/Kariqs/amexan-api/models"
	"github.com/Kariqs/amexan-api/utils"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	// Default cost for bcrypt password hashing
	bcryptCost = 10

	// Standard response messages
	msgInvalidInput          = "invalid input"
	msgUserAlreadyExists     = "user already exists"
	msgFailedToHashPassword  = "failed to hash password"
	msgInvalidCredentials    = "invalid username or password"
	msgAccountNotActivated   = "Account not activated, check your email to activate email."
	msgFailedToGenerateToken = "failed to generate token"
	msgInternalServerError   = "Internal server error"
	msgInvalidActivationLink = "Invalid or expired activation link"
	msgActivationSuccess     = "account has been activated successfully."
	msgResetLinkSent         = "Check your email for a password reset link."
	msgUserCreated           = "User created successfully. Check your email to activate your account."
	msgUserNotFound          = "user with this email does not exist"
	msgResetTokenError       = "There was an error trying to generate password reset link. Try again later."
	msgUnableToSaveToken     = "unable to save reset token."
	msgUnableToResetPassword = "unable to reset password"
)

func sendJSONResponse(ctx *gin.Context, status int, data gin.H) {
	ctx.JSON(status, data)
}

func sendErrorResponse(ctx *gin.Context, status int, message string) {
	sendJSONResponse(ctx, status, gin.H{"message": message})
}

func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func comparePasswords(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}

func generateJWT(user models.User) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  user.ID,
		"email":    user.Email,
		"username": user.Username,
		"role":     user.Role,
		"iat":      time.Now().Unix(),
		"exp":      time.Now().Add(time.Hour * 24 * 30).Unix(),
	})

	jwtSecret := os.Getenv("JWT_SECRET")
	return token.SignedString([]byte(jwtSecret))
}

func checkUserExists(email, username string) (bool, error) {
	var existingUser models.User
	result := initializers.DB.Where("email = ? OR username = ?", email, username).Find(&existingUser)
	return result.RowsAffected > 0, result.Error
}

func findUserByIdentifier(identifier string) (models.User, error) {
	var user models.User
	result := initializers.DB.Where("email = ? OR username = ?", identifier, identifier).First(&user)
	return user, result.Error
}

func findUserByEmail(email string) (models.User, error) {
	var user models.User
	result := initializers.DB.Where("email = ?", email).First(&user)
	return user, result.Error
}

// Send an account verification email
func sendAccountVerificationEmail(user models.User, activationToken string) error {
	emailData := utils.EmailData{
		Name:            user.Username,
		Message:         "Thank you for signing up! Click the button below to verify your account.",
		VerificationURL: os.Getenv("FRONTEND_URL") + "/auth/verify-email?token=" + url.QueryEscape(activationToken),
		LogoURL:         "https://www.amexan.store/images/logo.jpg",
	}

	templatePath := filepath.Join("templates", "verify_email.html")
	return utils.SendEmail(user.Email, "Account Verification", emailData, templatePath)
}

// Send a password reset email
func sendPasswordResetEmail(user models.User, resetToken string) error {
	emailData := utils.EmailData{
		Name:            user.Username,
		Message:         "You requested a password reset. Click the button below to reset your password.",
		VerificationURL: os.Getenv("FRONTEND_URL") + "/auth/reset-password?token=" + url.QueryEscape(resetToken),
		LogoURL:         "https://www.amexan.store/images/logo.jpg",
	}

	templatePath := filepath.Join("templates", "reset_password.html")
	return utils.SendEmail(user.Email, "Amexan Account Password Reset", emailData, templatePath)
}

// Signup handles user registration
func Signup(ctx *gin.Context) {
	var signUpData models.User
	if err := ctx.ShouldBindJSON(&signUpData); err != nil {
		sendErrorResponse(ctx, http.StatusBadRequest, msgInvalidInput)
		return
	}

	exists, err := checkUserExists(signUpData.Email, signUpData.Username)
	if err != nil {
		log.Println("Database error during user check:", err)
		sendErrorResponse(ctx, http.StatusInternalServerError, msgInternalServerError)
		return
	}
	if exists {
		sendErrorResponse(ctx, http.StatusBadRequest, msgUserAlreadyExists)
		return
	}

	// Hash the password
	hashedPassword, err := hashPassword(signUpData.Password)
	if err != nil {
		log.Println("Password hashing error:", err)
		sendErrorResponse(ctx, http.StatusInternalServerError, msgFailedToHashPassword)
		return
	}
	signUpData.Password = hashedPassword

	// Assign default role if not specified
	if signUpData.Role == "" {
		signUpData.Role = "user"
	}

	// Generate and assign activation token
	activationToken, err := utils.GenerateCode(16)
	if err != nil {
		log.Println("Token generation error:", err)
		sendErrorResponse(ctx, http.StatusInternalServerError, msgInternalServerError)
		return
	}
	signUpData.AccountActivationToken = activationToken
	signUpData.AccountActivated = false

	// Create the user in the database
	if result := initializers.DB.Create(&signUpData); result.Error != nil {
		log.Println("User creation error:", result.Error)
		sendErrorResponse(ctx, http.StatusInternalServerError, msgInternalServerError)
		return
	}

	// Send email to the user
	if err := sendAccountVerificationEmail(signUpData, activationToken); err != nil {
		log.Println("Error sending verification email:", err)
		// Continue despite email error, but log it
	} else {
		log.Println("Verification email sent successfully to:", signUpData.Email)
	}

	sendJSONResponse(ctx, http.StatusCreated, gin.H{"message": msgUserCreated})
}

// Login handles user authentication
func Login(ctx *gin.Context) {
	var loginData models.LoginData
	if err := ctx.ShouldBindJSON(&loginData); err != nil {
		sendErrorResponse(ctx, http.StatusBadRequest, msgInvalidInput)
		return
	}

	// Find the user
	user, err := findUserByIdentifier(loginData.Identifier)
	if err != nil {
		sendErrorResponse(ctx, http.StatusBadRequest, msgInvalidCredentials)
		return
	}

	// Check if the password is correct
	if err := comparePasswords(user.Password, loginData.Password); err != nil {
		sendErrorResponse(ctx, http.StatusBadRequest, msgInvalidCredentials)
		return
	}

	// Check if account is activated
	if !user.AccountActivated {
		sendErrorResponse(ctx, http.StatusBadRequest, msgAccountNotActivated)
		return
	}

	// Generate a JWT token
	tokenString, err := generateJWT(user)
	if err != nil {
		log.Println("JWT generation error:", err)
		sendErrorResponse(ctx, http.StatusInternalServerError, msgFailedToGenerateToken)
		return
	}

	sendJSONResponse(ctx, http.StatusOK, gin.H{"token": tokenString})
}

// ActivateAccount activates a user account using the activation token
func ActivateAccount(ctx *gin.Context) {
	activationToken := ctx.Param("activationToken")

	result := initializers.DB.Model(&models.User{}).
		Where("account_activation_token = ?", activationToken).
		Updates(map[string]any{
			"account_activated":        true,
			"account_activation_token": "",
		})

	if result.Error != nil {
		log.Println("Account activation error:", result.Error)
		sendErrorResponse(ctx, http.StatusInternalServerError, msgInternalServerError)
		return
	}

	if result.RowsAffected == 0 {
		sendErrorResponse(ctx, http.StatusBadRequest, msgInvalidActivationLink)
		return
	}

	sendJSONResponse(ctx, http.StatusOK, gin.H{"message": msgActivationSuccess})
}

// SendPasswordResetLink sends a password reset link to the user's email
func SendPasswordResetLink(ctx *gin.Context) {
	type ForgotPasswordBody struct {
		Email string `json:"email" binding:"required,email"`
	}

	var forgotPasswordData ForgotPasswordBody
	if err := ctx.ShouldBindJSON(&forgotPasswordData); err != nil {
		log.Println("Bind error:", err)
		log.Printf("Raw request body: %+v\n", forgotPasswordData)
		sendErrorResponse(ctx, http.StatusBadRequest, msgInvalidInput)
		return
	}

	// Find the user
	user, err := findUserByEmail(forgotPasswordData.Email)
	if err != nil {
		sendErrorResponse(ctx, http.StatusBadRequest, msgUserNotFound)
		return
	}

	// Generate password reset token
	passwordResetToken, err := utils.GenerateCode(16)
	if err != nil {
		log.Println("Reset token generation error:", err)
		sendErrorResponse(ctx, http.StatusInternalServerError, msgResetTokenError)
		return
	}

	// Save the reset token to db
	if result := initializers.DB.Model(&models.User{}).
		Where("email = ?", forgotPasswordData.Email).
		Update("password_reset_token", passwordResetToken); result.Error != nil {

		log.Println("Error saving reset token:", result.Error)
		sendErrorResponse(ctx, http.StatusInternalServerError, msgUnableToSaveToken)
		return
	}

	// Send email to the user
	if err := sendPasswordResetEmail(user, passwordResetToken); err != nil {
		log.Println("Error sending password reset email:", err)
	} else {
		log.Println("Password reset email sent successfully to:", forgotPasswordData.Email)
	}

	sendJSONResponse(ctx, http.StatusOK, gin.H{"message": msgResetLinkSent})
}

// ResetPassword resets a user's password using a reset token
func ResetPassword(ctx *gin.Context) {
	type ResetPasswordInfo struct {
		Password string `json:"password" binding:"required,min=8"`
	}

	var resetPasswordData ResetPasswordInfo
	if err := ctx.ShouldBindJSON(&resetPasswordData); err != nil {
		log.Println("Invalid reset password data:", err)
		sendErrorResponse(ctx, http.StatusBadRequest, msgInvalidInput)
		return
	}

	// Hash the new password
	hashedPassword, err := hashPassword(resetPasswordData.Password)
	if err != nil {
		log.Println("Password hashing error:", err)
		sendErrorResponse(ctx, http.StatusInternalServerError, msgFailedToHashPassword)
		return
	}

	resetToken := ctx.Param("resetToken")
	result := initializers.DB.Model(&models.User{}).
		Where("password_reset_token = ?", resetToken).
		Updates(map[string]any{
			"password":             hashedPassword,
			"password_reset_token": "",
		})

	if result.Error != nil {
		log.Println("Error resetting password:", result.Error)
		sendErrorResponse(ctx, http.StatusInternalServerError, msgUnableToResetPassword)
		return
	}

	if result.RowsAffected == 0 {
		sendErrorResponse(ctx, http.StatusBadRequest, msgInvalidActivationLink)
		return
	}

	sendJSONResponse(ctx, http.StatusOK, gin.H{"message": "Password reset successful"})
}
