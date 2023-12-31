package utils

import (
	"os"
	"qexchange/models"
	"time"

	"github.com/golang-jwt/jwt"
)

func GenerateJWTToken(user models.User) (string, error) {
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":  user.ID,
		"exp": time.Now().Add(time.Hour * 72).Unix(),
		"adm": user.IsAdmin,
	})

	token, err := t.SignedString([]byte(os.Getenv("JWT_SECRET")))

	return token, err
}
