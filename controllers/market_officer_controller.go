package controllers

import (
	"backend/database"
	"backend/models"
	"net/http"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"

	"errors"
	"log"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v4"

	"gorm.io/gorm"
)

var jwtSecret = []byte(getJWTSecret())

func getJWTSecret() string {
	if os.Getenv("JWT_SECRET") != "" {
		return os.Getenv("JWT_SECRET")
	}
	return "default-secret" // fallback
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Success bool               `json:"success"`
	Message string             `json:"message"`
	Data    *LoginResponseData `json:"data,omitempty"`
}

type LoginResponseData struct {
	Officer *OfficerResponse `json:"officer"`
	Token   string           `json:"token"`
	Market  *MarketResponse  `json:"market"`
}

type OfficerResponse struct {
	ID       uint64          `json:"id"`
	Name     string          `json:"name"`
	Username string          `json:"username"`
	Nik      string          `json:"nik"`
	Phone    string          `json:"phone"`
	ImageURL string          `json:"image_url"`
	MarketID uint64          `json:"market_id"`
	Market   *MarketResponse `json:"market"`
}
type MarketResponse struct {
	ID        uint    `json:"id"`
	Name      string  `json:"name"`
	Location  string  `json:"location"`
	ImageURL  string  `json:"image_url"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func Login(c *fiber.Ctx) error {
	var req LoginRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(LoginResponse{
			Success: false,
			Message: "Format request tidak valid",
		})
	}

	if req.Username == "" || req.Password == "" {
		return c.Status(http.StatusBadRequest).JSON(LoginResponse{
			Success: false,
			Message: "Username dan password wajib diisi",
		})
	}

	var officer models.MarketOfficer
	result := database.DB.Preload("Market").Where("username = ?", req.Username).First(&officer)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return c.Status(http.StatusUnauthorized).JSON(LoginResponse{
				Success: false,
				Message: "Username atau password salah",
			})
		}
		return c.Status(http.StatusInternalServerError).JSON(LoginResponse{
			Success: false,
			Message: "Terjadi kesalahan saat mengakses database",
		})
	}

	// Validasi market assignment
	if officer.MarketID == 0 {
		return c.Status(http.StatusForbidden).JSON(LoginResponse{
			Success: false,
			Message: "Officer tidak memiliki pasar yang ditugaskan",
		})
	}

	if !officer.IsActive {
		return c.Status(http.StatusUnauthorized).JSON(LoginResponse{
			Success: false,
			Message: "Akun tidak aktif. Hubungi admin.",
		})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(officer.Password), []byte(req.Password)); err != nil {
		return c.Status(http.StatusUnauthorized).JSON(LoginResponse{
			Success: false,
			Message: "Username atau password salah",
		})
	}

	// Generate JWT token
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := jwt.MapClaims{
		"username":   officer.Username,
		"officer_id": officer.ID,
		"market_id":  officer.MarketID,
		"exp":        expirationTime.Unix(),
	}
	log.Printf("Creating token for officer %s with market_id %d", officer.Username, officer.MarketID)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		log.Printf("Gagal membuat token: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(LoginResponse{
			Success: false,
			Message: "Gagal membuat token login",
		})
	}

	officerResponse := &OfficerResponse{
		ID:       officer.ID,
		Name:     officer.Name,
		Username: officer.Username,
		Nik:      officer.Nik,
		Phone:    officer.Phone,
		ImageURL: officer.ImageURL,
		MarketID: officer.MarketID,
		Market: &MarketResponse{
			ID:        officer.Market.ID,
			Name:      officer.Market.Name,
			Location:  officer.Market.Location,
			ImageURL:  officer.Market.ImageURL,
			Latitude:  officer.Market.Latitude,
			Longitude: officer.Market.Longitude,
		},
	}

	return c.JSON(LoginResponse{
		Success: true,
		Message: "Login berhasil",
		Data: &LoginResponseData{
			Officer: officerResponse,
			Token:   tokenString,
			Market:  officerResponse.Market,
		},
	})
}

// Get all market officers
func GetMarketOfficers(c *fiber.Ctx) error {
	var officers []models.MarketOfficer
	database.DB.Preload("Market").Find(&officers)
	return c.JSON(officers)
}

func ToggleOfficerStatus(c *fiber.Ctx) error {
	id := c.Params("id")
	officerID, err := strconv.Atoi(id)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "ID tidak valid"})
	}

	var officer models.MarketOfficer
	result := database.DB.First(&officer, officerID)
	if result.Error != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Petugas tidak ditemukan"})
	}

	officer.IsActive = !officer.IsActive
	database.DB.Save(&officer)

	return c.JSON(fiber.Map{"message": "Status petugas diperbarui", "is_active": officer.IsActive})
}

// Get a single market officer by ID
func GetMarketOfficerByID(c *fiber.Ctx) error {
	id := c.Params("id")
	var officer models.MarketOfficer
	if err := database.DB.Preload("Market").First(&officer, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Market officer not found"})
	}
	return c.JSON(officer)
}

// Create a new market officer
func CreateMarketOfficer(c *fiber.Ctx) error {
	var officer models.MarketOfficer
	if err := c.BodyParser(&officer); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid input"})
	}

	// Check if MarketID exists
	var market models.Market
	// Periksa jika market soft-deleted
	err := database.DB.Unscoped().First(&market, officer.MarketID).Error
	if err != nil || market.DeletedAt.Valid {
		return c.Status(400).JSON(fiber.Map{"error": "Market not found or deleted"})
	}

	// Tambahkan di awal sebelum `DB.Create(...)`
	var existing models.MarketOfficer
	if err := database.DB.Where("nik = ?", officer.Nik).First(&existing).Error; err == nil {
		return c.Status(409).JSON(fiber.Map{"error": "NIK sudah digunakan."})
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(officer.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal mengenkripsi password"})
	}
	officer.Password = string(hashedPassword)

	if err := database.DB.Create(&officer).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create officer"})
	}

	return c.Status(201).JSON(fiber.Map{"message": "Market officer added", "officer": officer})
}

// Update market officer
func UpdateMarketOfficer(c *fiber.Ctx) error {
	id := c.Params("id")
	var officer models.MarketOfficer
	if err := database.DB.First(&officer, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Market officer not found"})
	}

	updateData := new(models.MarketOfficer)
	if err := c.BodyParser(updateData); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid input"})
	}

	officer.Name = updateData.Name
	officer.Nik = updateData.Nik
	officer.Phone = updateData.Phone
	// officer.PhotoURL = updateData.PhotoURL
	officer.MarketID = updateData.MarketID
	officer.Password = updateData.Password
	officer.Username = updateData.Username

	if updateData.Password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(updateData.Password), bcrypt.DefaultCost)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Gagal mengenkripsi password"})
		}
		officer.Password = string(hashedPassword)
	}

	if err := database.DB.Save(&officer).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update officer"})
	}

	return c.JSON(fiber.Map{"message": "Market officer updated", "officer": officer})
}

// Delete market officer
func DeleteMarketOfficer(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := database.DB.Delete(&models.MarketOfficer{}, id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete officer"})
	}
	return c.JSON(fiber.Map{"message": "Market officer deleted successfully"})
}
