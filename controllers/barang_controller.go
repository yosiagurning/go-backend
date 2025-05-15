package controllers

import (
	"backend/database"
	"backend/models"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
)

func GetAllBarang(c *fiber.Ctx) error {
	var barang []models.Barang
	if err := database.DB.Preload("Category").Find(&barang).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch barang"})
	}
	return c.JSON(barang)
}

func GetBarangByID(c *fiber.Ctx) error {
	id := c.Params("id")
	var barang models.Barang
	if err := database.DB.Preload("Category").First(&barang, "id_barang = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Barang not found"})
	}
	return c.JSON(barang)
}

func CreateBarang(c *fiber.Ctx) error {
	var barang models.Barang
	if err := c.BodyParser(&barang); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid input"})
	}

	// Set default values
	barang.HargaSebelumnya = 0
	barang.TanggalUpdate = time.Now()

	// Validate category exists
	var category models.Category
	if err := database.DB.First(&category, barang.CategoryID).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": fmt.Sprintf("Category ID %d not found", barang.CategoryID),
		})
	}

	// Calculate average price
	barang.HargaSekarang = (barang.HargaPedagang1 + barang.HargaPedagang2 + barang.HargaPedagang3) / 3

	// Start transaction
	tx := database.DB.Begin()

	if err := tx.Create(&barang).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create barang"})
	}

	// Sync with price table
	if err := SyncBarangWithPrice(barang.IdBarang, tx); err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to sync with price: %v", err)})
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to commit transaction"})
	}

	return c.Status(201).JSON(barang)
}

func UpdateBarang(c *fiber.Ctx) error {
	id := c.Params("id")
	var existingBarang models.Barang

	if err := database.DB.First(&existingBarang, "id_barang = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Barang not found"})
	}

	var input struct {
		Nama            string  `json:"nama"`
		Satuan          string  `json:"satuan"`
		HargaPedagang1  float64 `json:"harga_pedagang1"`
		HargaPedagang2  float64 `json:"harga_pedagang2"`
		HargaPedagang3  float64 `json:"harga_pedagang3"`
		CategoryID      uint64  `json:"category_id"`
		AlasanPerubahan string  `json:"alasan_perubahan"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid input format"})
	}

	// Start transaction
	tx := database.DB.Begin()

	// Validasi CategoryID
	if input.CategoryID != 0 {
		var category models.Category
		if err := tx.First(&category, input.CategoryID).Error; err != nil {
			tx.Rollback()
			return c.Status(400).JSON(fiber.Map{
				"error": fmt.Sprintf("Category ID %d not found", input.CategoryID),
			})
		}
		categoryID := uint(input.CategoryID)
		existingBarang.CategoryID = &categoryID
	} else {
		existingBarang.CategoryID = nil
	}

	// Update other fields
	existingBarang.Nama = input.Nama
	existingBarang.Satuan = input.Satuan
	existingBarang.HargaPedagang1 = input.HargaPedagang1
	existingBarang.HargaPedagang2 = input.HargaPedagang2
	existingBarang.HargaPedagang3 = input.HargaPedagang3
	existingBarang.AlasanPerubahan = input.AlasanPerubahan

	// Calculate new average price
	newPrice := (input.HargaPedagang1 + input.HargaPedagang2 + input.HargaPedagang3) / 3

	if newPrice != existingBarang.HargaSekarang {
		history := models.BarangHistory{
			BarangID:       existingBarang.IdBarang,
			HargaPedagang1: existingBarang.HargaPedagang1,
			HargaPedagang2: existingBarang.HargaPedagang2,
			HargaPedagang3: existingBarang.HargaPedagang3,
			HargaSekarang:  existingBarang.HargaSekarang,
			TanggalUpdate:  time.Now(),
		}

		if err := tx.Create(&history).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to save price history"})
		}

		existingBarang.HargaSebelumnya = existingBarang.HargaSekarang
		existingBarang.HargaSekarang = newPrice
	}

	existingBarang.TanggalUpdate = time.Now()

	if err := tx.Save(&existingBarang).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update barang"})
	}

	// Sync with price table
	if err := SyncBarangWithPrice(existingBarang.IdBarang, tx); err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to sync with price: %v", err)})
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to commit transaction"})
	}

	return c.JSON(existingBarang)
}

func DeleteBarang(c *fiber.Ctx) error {
	id := c.Params("id")
	fmt.Println("🧪 DELETE Request ID:", id)

	tx := database.DB.Begin()

	// Hapus history
	if err := tx.Unscoped().Where("barang_id = ?", id).Delete(&models.BarangHistory{}).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Gagal hapus history", "detail": err.Error()})
	}

	// Find the barang to get its name before deleting
	var barang models.Barang
	if err := tx.First(&barang, "id_barang = ?", id).Error; err == nil {
		// Delete corresponding price records
		if err := tx.Where("item_name = ?", barang.Nama).Delete(&models.Price{}).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Gagal hapus price terkait", "detail": err.Error()})
		}

		// Delete price history
		if err := tx.Where("item_name = ?", barang.Nama).Delete(&models.PriceHistory{}).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Gagal hapus price history terkait", "detail": err.Error()})
		}
	}

	// Hapus barang
	result := tx.Unscoped().Where("id_barang = ?", id).Delete(&models.Barang{})
	fmt.Println("📦 Rows affected:", result.RowsAffected)

	if result.Error != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Gagal hapus barang", "detail": result.Error.Error()})
	}
	if result.RowsAffected == 0 {
		tx.Rollback()
		return c.Status(404).JSON(fiber.Map{"error": "Barang tidak ditemukan"})
	}

	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal commit", "detail": err.Error()})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Barang berhasil dihapus permanen",
	})
}

// GetPriceHistory fetches price history for a barang
func GetBarangHistory(c *fiber.Ctx) error {
	id := c.Params("id")

	var history []models.BarangHistory
	if err := database.DB.
		Where("barang_id = ?", id).
		Order("tanggal_update DESC").
		Find(&history).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch price history"})
	}
	return c.JSON(history)
}

// GetBarangByMarketID menampilkan semua barang yang berasal dari pasar tertentu
// GetBarangByMarketID menampilkan semua barang yang berasal dari pasar tertentu
func GetBarangByMarketID(c *fiber.Ctx) error {
	marketID := c.Params("marketId")

	// First get all categories for this market
	var categories []models.Category
	if err := database.DB.
		Where("JSON_CONTAINS(market_ids, ?)", marketID).
		Find(&categories).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal mengambil kategori berdasarkan market"})
	}

	// If no categories found, return empty array
	if len(categories) == 0 {
		return c.JSON([]models.Barang{})
	}

	// Extract category IDs
	var categoryIDs []uint
	for _, category := range categories {
		categoryIDs = append(categoryIDs, category.ID)
	}

	// Get all products for these categories
	var barang []models.Barang
	if err := database.DB.
		Where("category_id IN ?", categoryIDs).
		Preload("Category").
		Find(&barang).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal mengambil data barang berdasarkan kategori"})
	}

	return c.JSON(barang)
}

func GetBarangByMarketIDPaginated(c *fiber.Ctx) error {
	marketID := c.Params("marketId")
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 10)

	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	offset := (page - 1) * limit

	var barang []models.Barang
	result := database.DB.
		Joins("JOIN categories ON categories.id = barangs.category_id").
		Where("categories.market_id = ?", marketID).
		Preload("Category").
		Limit(limit).
		Offset(offset).
		Find(&barang)

	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal mengambil data barang dengan pagination"})
	}

	return c.JSON(barang)
}
