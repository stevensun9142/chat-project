package auth

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID   string `json:"sub"`
	Username string `json:"username"`
}

type JWTValidator struct {
	secret []byte
}

func NewJWTValidator(secret string) *JWTValidator {
	return &JWTValidator{secret: []byte(secret)}
}

func (v *JWTValidator) keyFunc(t *jwt.Token) (interface{}, error) {
	if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
		return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
	}
	return v.secret, nil
}

func (v *JWTValidator) Validate(tokenStr string) (*Claims, error) {
	token, err := jwt.Parse(tokenStr, v.keyFunc)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	mapClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	sub, _ := mapClaims.GetSubject()
	username, _ := mapClaims["username"].(string)

	if sub == "" || username == "" {
		return nil, fmt.Errorf("missing required claims (sub, username)")
	}

	return &Claims{UserID: sub, Username: username}, nil
}
