package middleware

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
)

func JWTAdminMiddleware(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"message": "Token diperlukan dalam format Bearer",
		})
	}

	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("metode signing tidak valid: %v", token.Header["alg"])
		}
		return []byte(os.Getenv("JWT_SECRET")), nil
	})

	if err != nil || !token.Valid {
		log.Printf("❌ Token admin tidak valid: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"message": "Token tidak valid",
		})
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || claims["username"] == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"message": "Token tidak memiliki username",
		})
	}

	// Inject username ke context
	c.Locals("username", claims["username"].(string))

	log.Printf("✅ Admin login sebagai: %s", claims["username"].(string))

	return c.Next()
}
