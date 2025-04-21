package models

import "gorm.io/gorm"

type User struct {
	gorm.Model
	Fullname        string `json:"fullname"`
	Username        string `json:"username"`
	Email           string `json:"email"`
	Phone           int    `json:"phone"`
	Occupation      string `json:"occupation"`
	Password        string `json:"password"`
	Role            string `json:"role"`
	AcceptTerms     bool   `json:"acceptterms"`
	SubscribeToNews bool   `json:"subscribetonews"`
}

type LoginData struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
