package main

import (
	"backend/database"
	"backend/models"
	"backend/routes"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"
)

var jwtKey []byte

func getJWTSecret() string {
	if os.Getenv("JWT_SECRET") != "" {
		return os.Getenv("JWT_SECRET")
	}
	return "default-secret"
}

func init() {
	jwtKey = []byte(getJWTSecret())
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Success bool                  `json:"success"`
	Message string                `json:"message"`
	Data    *LoginResponsePayload `json:"data,omitempty"`
}

type LoginResponsePayload struct {
	Officer *models.MarketOfficer `json:"officer"`
	Token   string                `json:"token"`
}

func loginHandlermobile(c *fiber.Ctx) error {
	var creds LoginRequest
	if err := c.BodyParser(&creds); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(LoginResponse{
			Success: false,
			Message: "Invalid request format",
		})
	}

	var officer models.MarketOfficer
	result := database.DB.Preload("Market").Where("username = ?", creds.Username).First(&officer)
	if result.Error != nil {
		log.Println("❌ Officer not found:", creds.Username)
		return c.Status(fiber.StatusUnauthorized).JSON(LoginResponse{
			Success: false,
			Message: "Username atau password salah",
		})
	}

	if !officer.IsActive {
		return c.Status(fiber.StatusUnauthorized).JSON(LoginResponse{
			Success: false,
			Message: "Akun tidak aktif. Hubungi admin",
		})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(officer.Password), []byte(creds.Password)); err != nil {
		log.Println("❌ Invalid password for officer:", creds.Username)
		return c.Status(fiber.StatusUnauthorized).JSON(LoginResponse{
			Success: false,
			Message: "Username atau password salah",
		})
	}

	expirationTime := time.Now().Add(24 * time.Hour)
	claims := jwt.MapClaims{
		"username": officer.Username,
		"exp":      expirationTime.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtKey)
	if err != nil {
		log.Printf("❌ Error generating JWT token: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(LoginResponse{
			Success: false,
			Message: "Gagal membuat token login",
		})
	}

	return c.JSON(LoginResponse{
		Success: true,
		Message: "Login berhasil",
		Data: &LoginResponsePayload{
			Officer: &officer,
			Token:   tokenString,
		},
	})
}

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// 🔧 Fungsi untuk inisialisasi database
func initDatabase() {
	database.ConnectDatabase()

	if database.DB == nil {
		log.Fatalf("❌ Koneksi database nil! Pastikan database berjalan.")
		os.Exit(1)
	}

	fmt.Println("✅ Database sudah siap digunakan!")
}

// 🔐 Fungsi untuk menangani login dengan hashing password
func loginHandler(c *fiber.Ctx) error {
	var creds Credentials
	if err := c.BodyParser(&creds); err != nil {
		log.Println("❌ Error parsing request body:", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request format"})
	}

	var user models.User
	result := database.DB.Where("username = ?", creds.Username).First(&user)
	if result.Error != nil {
		log.Println("❌ User not found:", creds.Username)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid username or password"})
	}

	// Validasi password dengan bcrypt
	err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(creds.Password))
	if err != nil {
		log.Println("❌ Invalid password for user:", creds.Username)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid username or password"})
	}

	// 🕒 Buat token JWT dengan masa berlaku 24 jam
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &Claims{
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtKey)
	if err != nil {
		log.Println("❌ Error generating JWT token:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not generate token"})
	}

	// Kirim response dengan token
	return c.JSON(fiber.Map{"token": tokenString, "user": user.Username})
}

func main() {
	// Inisialisasi database
	initDatabase()

	// Inisialisasi Fiber
	app := fiber.New()

	// 🛡 Middleware CORS & Logger
	app.Use(cors.New(cors.Config{
		AllowOrigins: "http://localhost:8000,http://yourdomain.com", // Bisa disesuaikan dengan domain tertentu jika perlu
		AllowMethods: "GET, POST, PUT, DELETE, OPTIONS",
		AllowHeaders: "Content-Type, Authorization",
	}))
	app.Use(logger.New()) // Tambahkan logger untuk debugging request

	// Daftarkan Routes
	routes.RegisterPriceRoutes(app)
	routes.RegisterMarketRoutes(app)
	routes.RegisterCategoryRoutes(app)
	routes.RegisterMarketOfficerRoutes(app)
	routes.RegisterBarangRoutes(app)
	routes.SetupRoutes(app)
	routes.RegisterSyncRoutes(app)

	web := app.Group("/api")
	web.Post("/login", loginHandler)

	// Mobile routes
	mobile := app.Group("/auth")
	mobile.Post("/login", loginHandlermobile)

	// Endpoint testing
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "🚀 Golang Backend is Running!"})
	})

	// Jalankan server di port 8081
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081" // fallback jika tidak di Railway
	}
	fmt.Println("🚀 Server running on port " + port)
	log.Fatal(app.Listen(":" + port))
	

	fmt.Printf("🌍 Running in environment: %s\n", os.Getenv("APP_ENV"))

}
