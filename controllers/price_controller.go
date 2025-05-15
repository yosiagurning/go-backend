package controllers

import (
	"backend/database"
	"backend/models"
	"time"

	"fmt"

	"github.com/gofiber/fiber/v2"
)

func GetPrices(c *fiber.Ctx) error {
	marketID := c.Query("market_id")
	categoryID := c.Query("category_id")

	var prices []models.Price
	query := database.DB.Preload("Market").Preload("Category")

	if search := c.Query("search"); search != "" {
		query = query.Where("item_name LIKE ?", "%"+search+"%")
	}

	switch c.Query("direction") {
	case "naik":
		query = query.Where("current_price > initial_price")
	case "turun":
		query = query.Where("current_price < initial_price")
	}

	switch c.Query("range") {
	case "murah":
		query = query.Where("current_price < ?", 10000)
	case "sedang":
		query = query.Where("current_price BETWEEN ? AND ?", 10000, 50000)
	case "mahal":
		query = query.Where("current_price > ?", 50000)
	}

	if marketID != "" {
		query = query.Where("market_id = ?", marketID)
	}
	if categoryID != "" {
		query = query.Where("category_id = ?", categoryID)
	}

	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	if startDate != "" && endDate != "" {
		query = query.Where("updated_at BETWEEN ? AND ?", startDate+" 00:00:00", endDate+" 23:59:59")
	}

	if err := query.Find(&prices).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal mengambil data harga"})
	}

	fmt.Printf("✅ Jumlah data harga: %d\n", len(prices))

	return c.JSON(prices)
}
func GetPriceByID(c *fiber.Ctx) error {
	id := c.Params("id")
	var price models.Price
	if err := database.DB.First(&price, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Price not found"})
	}

	var prices []models.Price

	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	query := database.DB.Model(&models.Price{})

	if startDate != "" && endDate != "" {
		query = query.Where("updated_at BETWEEN ? AND ?", startDate+" 00:00:00", endDate+" 23:59:59")
	}

	if err := query.Find(&prices).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal mengambil data harga"})
	}

	return c.JSON(price)
}

func CreatePrice(c *fiber.Ctx) error {
	var price models.Price
	if err := c.BodyParser(&price); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid input", "detail": err.Error()})
	}

	// Start transaction
	tx := database.DB.Begin()

	// 🔍 Cek apakah sudah pernah ada barang dengan nama yang sama
	var existingItem models.Price
	if err := tx.Where("item_name = ?", price.ItemName).First(&existingItem).Error; err == nil {
		// Barang sudah pernah diinput → pakai item_id yang sama
		price.ItemID = existingItem.ItemID
	} else {
		// Barang belum pernah diinput → item_id = ID baru
		var lastItem models.Price
		tx.Order("item_id DESC").First(&lastItem)
		price.ItemID = lastItem.ItemID + 1
	}

	// 💡 Hitung persentase perubahan harga dengan aman
	if price.InitialPrice > 0 {
		price.ChangePercent = ((price.CurrentPrice - price.InitialPrice) / price.InitialPrice) * 100
	} else {
		price.ChangePercent = 0 // Untuk harga awal 0, set persentase perubahan ke 0
	}

	if err := tx.Create(&price).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create price"})
	}

	// Tambahkan histori
	history := models.PriceHistory{
		ItemID:        price.ItemID,
		ItemName:      price.ItemName,
		InitialPrice:  price.InitialPrice,
		CurrentPrice:  price.CurrentPrice,
		Reason:        price.Reason,
		MarketID:      price.MarketID,
		CategoryID:    price.CategoryID,
		ChangePercent: price.ChangePercent,
		CreatedAt:     time.Now(),
	}
	if err := tx.Create(&history).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create price history"})
	}

	// Sync with barang table
	if err := SyncPriceWithBarang(price.ID, tx); err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to sync with barang: %v", err)})
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to commit transaction"})
	}

	fmt.Printf("✅ Harga baru ditambahkan: %+v\n", price)

	return c.Status(201).JSON(price)
}

func UpdatePrice(c *fiber.Ctx) error {

	id := c.Params("id")
	var price models.Price

	now := time.Now()
	resetTime := time.Date(now.Year(), now.Month(), now.Day(), 8, 0, 0, 0, now.Location())

	var lastUpdate time.Time = price.UpdatedAt
	if lastUpdate.After(resetTime) && now.Before(resetTime.Add(24*time.Hour)) {
		jamTersisa := 24 - now.Sub(resetTime).Hours()
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": fmt.Sprintf("Data hanya bisa diedit sekali sehari. Coba lagi dalam %.0f jam.", jamTersisa),
		})
	}

	if err := database.DB.First(&price, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Price not found"})
	}

	var input models.Price
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid input"})
	}

	// Start transaction
	tx := database.DB.Begin()

	price.ItemName = input.ItemName
	price.Reason = input.Reason

	price.InitialPrice = price.CurrentPrice
	price.CurrentPrice = input.CurrentPrice

	if price.InitialPrice > 0 {
		price.ChangePercent = ((price.CurrentPrice - price.InitialPrice) / price.InitialPrice) * 100
	} else {
		price.ChangePercent = 0 // Untuk harga awal 0, set persentase perubahan ke 0
	}

	if err := tx.Save(&price).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update price"})
	}

	// Tambahkan histori
	history := models.PriceHistory{
		ItemID:        price.ItemID,
		ItemName:      price.ItemName,
		InitialPrice:  price.InitialPrice,
		CurrentPrice:  price.CurrentPrice,
		Reason:        price.Reason,
		MarketID:      price.MarketID,
		CategoryID:    price.CategoryID,
		ChangePercent: price.ChangePercent,
		CreatedAt:     time.Now(),
	}
	if err := tx.Create(&history).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create price history"})
	}

	// Sync with barang table
	if err := SyncPriceWithBarang(price.ID, tx); err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to sync with barang: %v", err)})
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to commit transaction"})
	}

	return c.JSON(price)
}

func DeletePrice(c *fiber.Ctx) error {
	id := c.Params("id")

	// Start transaction
	tx := database.DB.Begin()

	// Find the price to get its name before deleting
	var price models.Price
	if err := tx.First(&price, id).Error; err == nil {
		// Delete corresponding barang records if they exist only for this price
		var count int64
		tx.Model(&models.Price{}).Where("item_name = ? AND id != ?", price.ItemName, id).Count(&count)

		if count == 0 {
			// This is the only price record for this item, so we can delete the barang too
			if err := tx.Where("nama = ?", price.ItemName).Delete(&models.Barang{}).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{"error": "Failed to delete related barang", "detail": err.Error()})
			}

			// Delete barang history
			if err := tx.Where("barang_id IN (SELECT id_barang FROM barangs WHERE nama = ?)", price.ItemName).Delete(&models.BarangHistory{}).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{"error": "Failed to delete related barang history", "detail": err.Error()})
			}
		}
	}

	// Delete price history
	if err := tx.Where("item_id = ?", price.ItemID).Delete(&models.PriceHistory{}).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete price history", "detail": err.Error()})
	}

	// Delete the price
	if err := tx.Delete(&models.Price{}, id).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete price", "detail": err.Error()})
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to commit transaction"})
	}

	return c.JSON(fiber.Map{"message": "Price deleted successfully"})
}

func GetPriceHistory(c *fiber.Ctx) error {
	id := c.Params("id")

	if id == "" {
		return c.Status(400).JSON(fiber.Map{"error": "ID tidak boleh kosong"})
	}

	var prices []models.Price

	if err := database.DB.
		Where("item_id = ?", id).
		Order("updated_at ASC").
		Find(&prices).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal ambil data harga"})
	}

	if len(prices) == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "Harga tidak ditemukan untuk barang ini"})
	}

	var filteredPrices []models.Price
	var lastPrice float64 = -1

	for _, p := range prices {
		if p.CurrentPrice != lastPrice {
			filteredPrices = append(filteredPrices, p)
			lastPrice = p.CurrentPrice
		}
	}

	return c.JSON(filteredPrices)
}

func GetDashboardData(c *fiber.Ctx) error {
	var prices []models.Price

	if err := database.DB.Find(&prices).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal mengambil data harga"})
	}

	// Total komoditas (item unik)
	uniqueItems := make(map[string]bool)
	totalStockValue := 0.0
	var priceChanges []map[string]interface{}

	for _, p := range prices {
		uniqueItems[p.ItemName] = true
		totalStockValue += p.CurrentPrice

		priceChanges = append(priceChanges, map[string]interface{}{
			"item_name":      p.ItemName,
			"initial_price":  p.InitialPrice,
			"current_price":  p.CurrentPrice,
			"change_percent": p.ChangePercent,
			"change_date":    p.UpdatedAt.Format(time.RFC3339), // contoh hasil: "2025-05-12T09:42:01Z"

			"market":   p.Market.Name,
			"category": p.Category.Name,
		})
	}

	totalCommodities := len(uniqueItems)

	return c.JSON(fiber.Map{
		"total_commodities": totalCommodities,
		"total_stock_value": totalStockValue,
		"price_changes":     priceChanges,
	})

}

func GetPriceHistoryByItem(c *fiber.Ctx) error {
	itemID := c.Params("item_id")

	var histories []models.PriceHistory
	if err := database.DB.
		Where("item_id = ?", itemID).
		Order("created_at ASC").
		Find(&histories).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal ambil histori harga"})
	}

	return c.JSON(histories)
}

func GetPriceHistoryByCategory(c *fiber.Ctx) error {
	categoryID := c.Params("category_id")
	if categoryID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Kategori ID kosong"})
	}

	var rawHistories []models.PriceHistory
	if err := database.DB.
		Where("category_id = ?", categoryID).
		Order("created_at ASC").
		Find(&rawHistories).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal ambil histori harga berdasarkan kategori"})
	}

	// Filter: hanya ambil harga terakhir dari setiap item_id untuk setiap tanggal
	type DateItemKey struct {
		Date   string
		ItemID uint
	}

	latestPerDateItem := make(map[DateItemKey]models.PriceHistory)

	for _, h := range rawHistories {
		date := h.CreatedAt.Format("2006-01-02")
		key := DateItemKey{Date: date, ItemID: h.ItemID}

		// Simpan hanya histori terakhir (karena urutan ASC, ini otomatis replace hingga yang terakhir)
		latestPerDateItem[key] = h
	}

	// Gabungkan hasilnya menjadi slice
	var filteredHistories []models.PriceHistory
	for _, h := range latestPerDateItem {
		filteredHistories = append(filteredHistories, h)
	}

	return c.JSON(filteredHistories)
}
