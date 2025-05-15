package controllers

import (
	"backend/database"
	"backend/models"
	"log"
	"strconv"

	"gorm.io/gorm"

	"github.com/gofiber/fiber/v2"
)

// Ambil semua kategori
func GetCategories(c *fiber.Ctx) error {
	var categories []models.Category
	if err := database.DB.
		Preload("Markets"). // << INI WAJIB
		Find(&categories).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch categories"})
	}
	return c.JSON(categories)
}

// Get categories by market
func GetCategoriesByMarket(c *fiber.Ctx) error {
	marketID, err := strconv.ParseUint(c.Params("market_id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid market ID"})
	}

	// Validasi user memiliki akses ke market ini
	userMarketID := c.Locals("market_id").(uint64)
	if userMarketID != marketID {
		return c.Status(403).JSON(fiber.Map{"error": "Unauthorized access"})
	}

	var categories []models.Category
	if err := database.DB.
		Joins("JOIN category_markets ON categories.id = category_markets.category_id").
		Where("category_markets.market_id = ?", marketID).
		Find(&categories).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database error"})
	}

	return c.JSON(categories)
}

func GetCategoriesByMarketID(c *fiber.Ctx) error {
	marketID := c.Params("market_id")

	var categories []models.Category
	if err := database.DB.
		Joins("JOIN category_markets ON categories.id = category_markets.category_id").
		Where("category_markets.market_id = ?", marketID).
		Find(&categories).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database error"})
	}

	return c.JSON(categories)
}

// Ambil kategori berdasarkan ID
func GetCategoryByID(c *fiber.Ctx) error {
	id := c.Params("id")

	var category models.Category
	if err := database.DB.Preload("Markets").First(&category, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Category not found"})
	}

	var marketIDs []uint
	for _, market := range category.Markets {
		marketIDs = append(marketIDs, market.ID)
	}

	return c.JSON(fiber.Map{
		"id":          category.ID,
		"name":        category.Name,
		"description": category.Description,
		"market_ids":  marketIDs,
	})
}

// Tambah kategori baru
func CreateCategory(c *fiber.Ctx) error {
	type CategoryInput struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		MarketIDs   []uint `json:"market_ids"`
	}

	var input CategoryInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid input"})
	}

	// 🔴 Pindahkan validasi DUPLIKAT ke atas, sebelum INSERT!
	var existing models.Category
	if err := database.DB.
		Where("LOWER(name) = LOWER(?)", input.Name).
		First(&existing).Error; err == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Nama kategori sudah ada"})
	} else if err != gorm.ErrRecordNotFound {
		return c.Status(500).JSON(fiber.Map{"error": "Error checking existing category"})
	}

	// ✅ Jika aman, baru simpan kategori
	category := models.Category{
		Name:        input.Name,
		Description: input.Description,
	}

	if err := database.DB.Create(&category).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Nama kategori sudah digunakan"})
	}

	// Simpan relasi ke pasar
	for _, marketID := range input.MarketIDs {
		database.DB.Create(&models.CategoryMarket{
			CategoryID: category.ID,
			MarketID:   marketID,
		})
	}

	return c.Status(201).JSON(category)
}

// Update kategori berdasarkan ID
func UpdateCategory(c *fiber.Ctx) error {

	type CategoryInput struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		MarketIDs   []uint `json:"market_ids"`
	}

	id := c.Params("id")
	var category models.Category

	if err := database.DB.First(&category, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Category not found"})
	}

	var input CategoryInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid input"})
	}

	category.Name = input.Name
	category.Description = input.Description

	if err := database.DB.Save(&category).Error; err != nil {
		log.Printf("❌ Gagal menyimpan kategori ID %v: %v\n", category.ID, err) // ✅ log error nyata
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update category"})
	}

	log.Printf("📥 Parsed input: %+v", input)
	log.Printf("✅ Parsed market IDs: %+v", input.MarketIDs)

	// Hapus relasi lama, simpan ulang yang baru
	database.DB.Where("category_id = ?", category.ID).Delete(&models.CategoryMarket{})
	for _, marketID := range input.MarketIDs {
		database.DB.Create(&models.CategoryMarket{
			CategoryID: category.ID,
			MarketID:   marketID,
		})
	}

	// Cek apakah nama kategori sudah digunakan oleh kategori lain
	var existing models.Category
	if err := database.DB.
		Where("LOWER(name) = LOWER(?) AND id != ?", input.Name, category.ID).
		First(&existing).Error; err == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Nama kategori sudah digunakan"})
	}

	log.Printf("📥 Raw body: %v", c.Body())

	return c.JSON(category)
}

// Hapus kategori berdasarkan ID
func DeleteCategory(c *fiber.Ctx) error {
	id := c.Params("id")

	// Hapus relasi ke markets dulu
	if err := database.DB.Where("category_id = ?", id).Delete(&models.CategoryMarket{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal menghapus relasi pasar"})
	}

	// Opsional: hapus relasi harga jika ada (hati-hati jika digunakan oleh entitas lain)
	if err := database.DB.Where("category_id = ?", id).Delete(&models.Price{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal menghapus harga terkait"})
	}

	// Baru hapus kategori
	if err := database.DB.Delete(&models.Category{}, id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal menghapus kategori"})
	}

	return c.JSON(fiber.Map{"message": "Kategori berhasil dihapus"})
}
