package middleware

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
)

var jwtSecret = []byte(getJWTSecret())

func getJWTSecret() string {
	if os.Getenv("JWT_SECRET") != "" {
		return os.Getenv("JWT_SECRET")
	}
	return "default-secret"
}

func JWTMiddleware(c *fiber.Ctx) error {
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
		return jwtSecret, nil
	})

	if err != nil {
		log.Printf("Error parsing token: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"message": "Token tidak valid",
		})
	}

	if !token.Valid {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"message": "Token sudah kedaluwarsa",
		})
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"message": "Format token tidak valid",
		})
	}

	// Validasi claims penting
	requiredClaims := []string{"market_id", "officer_id", "username"}
	for _, claim := range requiredClaims {
		if _, ok := claims[claim]; !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"message": fmt.Sprintf("Token tidak mengandung %s", claim),
			})
		}
	}

	// Log claims untuk debugging
	log.Printf("JWT Claims - MarketID: %v, OfficerID: %v, Username: %v",
		claims["market_id"], claims["officer_id"], claims["username"])

	// Inject ke context
	c.Locals("market_id", uint64(claims["market_id"].(float64)))
	c.Locals("officer_id", uint64(claims["officer_id"].(float64)))
	c.Locals("username", claims["username"].(string))

	return c.Next()
}
func ValidateMarketAccess(c *fiber.Ctx) error {
	userMarketID := c.Locals("market_id").(uint64)
	requestMarketID, err := strconv.ParseUint(c.Params("market_id"), 10, 64)

	if err != nil || userMarketID != requestMarketID {
		return c.Status(403).JSON(fiber.Map{
			"error": "Akses ditolak untuk market ini",
		})
	}
	return c.Next()
}
