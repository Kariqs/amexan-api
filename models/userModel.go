package models

import "gorm.io/gorm"

type User struct {
	gorm.Model
	Fullname        string `json:"fullname"`
	Username        string `json:"username"`
	Email           string `json:"email"`
	Phone           string `json:"phone"`
	Occupation      string `json:"occupation"`
	Password        string `json:"password"`
	Role            string `json:"role"`
	AcceptTerms     bool   `json:"acceptTerms"`
	SubscribeToNews bool   `json:"subscribeToNews"`
}

type LoginData struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
