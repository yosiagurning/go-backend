package controllers

import (
	"backend/database"
	"backend/models"
	"fmt"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// Ambil semua pasar dengan opsi pencarian berdasarkan nama
func GetMarkets(c *fiber.Ctx) error {
	if database.DB == nil {
		fmt.Println("Database connection is nil!")
		return c.Status(500).JSON(fiber.Map{"error": "Database connection error"})
	}

	var markets []models.Market
	search := c.Query("search") // Ambil parameter search dari URL

	query := database.DB

	// Jika ada pencarian, filter berdasarkan nama
	if search != "" {
		search = strings.ToLower(search)
		query = query.Where("name LIKE ?", "%"+search+"%")
	}

	if err := database.DB.Find(&markets).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Gagal mengambil data pasar",
		})
	}

	// Ambil data dari database
	result := query.Find(&markets)
	if result.Error != nil {
		fmt.Println("Database Error:", result.Error)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to retrieve markets"})
	}

	// Jika tidak ada hasil, beri respons kosong
	if len(markets) == 0 {
		return c.JSON([]models.Market{})
	}

	return c.JSON(markets)
}

// Ambil pasar berdasarkan ID
func GetMarketByID(c *fiber.Ctx) error {
	id := c.Params("id")

	if database.DB == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database connection error"})
	}

	var market models.Market
	if err := database.DB.First(&market, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Market not found"})
	}
	return c.JSON(market)
}

// Buat pasar baru dengan validasi
func CreateMarket(c *fiber.Ctx) error {
	market := new(models.Market)

	if err := c.BodyParser(market); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid input"})
	}

	if market.Name == "" || market.Location == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name and location are required"})
	}

	var existing models.Market
	if err := database.DB.
		Where("LOWER(name) = LOWER(?)", market.Name).
		First(&existing).Error; err == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Nama pasar sudah ada"})
	}

	if database.DB == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database connection error"})
	}

	result := database.DB.Create(market)
	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create market"})
	}

	return c.Status(201).JSON(fiber.Map{"message": "Market added", "market": market})
}

// Perbarui data pasar berdasarkan ID dengan validasi
func UpdateMarket(c *fiber.Ctx) error {
	id := c.Params("id")
	var market models.Market

	if database.DB == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database connection error"})
	}

	if err := database.DB.First(&market, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Market not found"})
	}

	updateData := new(models.Market)
	if err := c.BodyParser(updateData); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid input"})
	}

	if updateData.Name != "" {
		market.Name = updateData.Name
	}
	if updateData.Location != "" {
		market.Location = updateData.Location
	}

	if updateData.ImageURL != "" {
		market.ImageURL = updateData.ImageURL
	}

	// Validasi jika nama baru sudah digunakan pasar lain
var conflict models.Market
if err := database.DB.
    Where("LOWER(name) = LOWER(?) AND id != ?", updateData.Name, id).
    First(&conflict).Error; err == nil {
    return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Nama pasar sudah digunakan"})
}


	result := database.DB.Save(&market)
	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update market"})
	}

	return c.JSON(fiber.Map{"message": "Market updated", "market": market})
}

// Perbarui lokasi pasar
// UpdateMarketLocation mengupdate koordinat lokasi pasar
func UpdateMarketLocation(c *fiber.Ctx) error {
	id := c.Params("id")

	// Struktur input untuk menerima data koordinat
	type LocationUpdate struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	}

	var input LocationUpdate
	if err := c.BodyParser(&input); err != nil {
		fmt.Println("❌ Body Parsing Error:", err)
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid input format",
		})
	}
	fmt.Println("✅ Received data:", input)

	// Validasi: Latitude dan Longitude tidak boleh nol
	if input.Latitude == 0 || input.Longitude == 0 {
		fmt.Println("❌ Invalid Latitude/Longitude values")
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Latitude and Longitude are required",
		})
	}

	// Cari pasar berdasarkan ID
	var market models.Market
	if err := database.DB.First(&market, id).Error; err != nil {
		fmt.Println("Market Not Found in DB:", err)
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "Market not found",
		})
	}
	fmt.Printf("Market Found: %+v\n", market) // Pastikan pasar ditemukan sebelum update

	// Update koordinat pasar
	market.Latitude = input.Latitude
	market.Longitude = input.Longitude

	// Simpan ke database dengan `Updates()`
	if err := database.DB.Model(&market).Updates(models.Market{
		Latitude:  input.Latitude,
		Longitude: input.Longitude,
	}).Error; err != nil {
		fmt.Println("❌ Database Save Error:", err)
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update market location",
		})
	}

	fmt.Println("✅ Market Updated:", market)

	err := database.DB.Model(&market).Updates(map[string]interface{}{
		"latitude":  input.Latitude,
		"longitude": input.Longitude,
	}).Error
	if err != nil {
		fmt.Println("Database Update Error:", err)
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update market location",
		})
	}

	// Tambahkan log untuk memastikan perubahan tersimpan
	rowsAffected := database.DB.RowsAffected
	if rowsAffected == 0 {
		fmt.Println("Warning: No rows affected, data might not be updated!")
	}

	fmt.Printf("Received Update Request: ID=%s, Lat=%f, Lng=%f\n", id, input.Latitude, input.Longitude)

	// Response sukses
	return c.JSON(fiber.Map{
		"message":   "Market location updated successfully",
		"market_id": market.ID,
		"latitude":  market.Latitude,
		"longitude": market.Longitude,
	})
}

// Hapus pasar berdasarkan ID
func DeleteMarket(c *fiber.Ctx) error {
	id := c.Params("id")

	if database.DB == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database connection error"})
	}

	if err := database.DB.Delete(&models.Market{}, id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete market"})
	}

	return c.JSON(fiber.Map{"message": "Market deleted successfully"})
}
