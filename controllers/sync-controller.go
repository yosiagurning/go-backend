package controllers

import (
	"backend/database"
	"backend/models"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// SyncBarangAndPrice synchronizes data between barang and price tables
func SyncBarangAndPrice(c *fiber.Ctx) error {
	// Get all barang items that need syncing
	var barangItems []models.Barang
	if err := database.DB.Find(&barangItems).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch barang items"})
	}

	// Get all price items
	var priceItems []models.Price
	if err := database.DB.Find(&priceItems).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch price items"})
	}

	// Create maps for easier lookup
	priceMap := make(map[string]models.Price)
	for _, price := range priceItems {
		priceMap[price.ItemName] = price
	}

	barangMap := make(map[string]models.Barang)
	for _, barang := range barangItems {
		barangMap[barang.Nama] = barang
	}

	// Start a transaction
	tx := database.DB.Begin()

	// Sync from barang to price
	for _, barang := range barangItems {
		if price, exists := priceMap[barang.Nama]; exists {
			// If price exists but values are different, update price
			if price.CurrentPrice != barang.HargaSekarang {
				price.InitialPrice = price.CurrentPrice
				price.CurrentPrice = barang.HargaSekarang
				if price.InitialPrice > 0 {
					price.ChangePercent = ((price.CurrentPrice - price.InitialPrice) / price.InitialPrice) * 100
				} else {
					price.ChangePercent = 0 // atau nilai lain yang sesuai
				}
				price.Reason = "Synchronized from mobile app"
				price.UpdatedAt = time.Now()

				if err := tx.Save(&price).Error; err != nil {
					tx.Rollback()
					return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to update price for %s: %v", barang.Nama, err)})
				}

				// Create price history
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
					return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to create price history for %s: %v", barang.Nama, err)})
				}
			}
		} else {
			// If price doesn't exist, create a new price entry
			// Find a suitable market and category ID
			var marketID, categoryID uint = 1, 1 // Default values
			if barang.CategoryID != nil {
				categoryID = uint(*barang.CategoryID)

				// Try to find a market associated with this category
				var categoryMarket models.CategoryMarket
				if err := tx.Where("category_id = ?", categoryID).First(&categoryMarket).Error; err == nil {
					marketID = categoryMarket.MarketID
				}
			}

			newPrice := models.Price{
				ItemID:       uint(barang.IdBarang),
				ItemName:     barang.Nama,
				InitialPrice: barang.HargaSebelumnya,
				CurrentPrice: barang.HargaSekarang,
				Reason:       "Created from mobile app data",
				MarketID:     marketID,
				CategoryID:   categoryID,
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			}

			// Hitung persentase perubahan dengan aman (hindari pembagian dengan nol)
			if barang.HargaSebelumnya > 0 {
				newPrice.ChangePercent = ((barang.HargaSekarang - barang.HargaSebelumnya) / barang.HargaSebelumnya) * 100
			} else {
				// Untuk barang baru atau harga sebelumnya 0, set persentase perubahan ke 0 atau nilai lain yang sesuai
				newPrice.ChangePercent = 0 // atau 100 jika ingin menandai sebagai barang baru
			}

			if err := tx.Create(&newPrice).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to create price for %s: %v", barang.Nama, err)})
			}

			// Create price history
			history := models.PriceHistory{
				ItemID:        newPrice.ItemID,
				ItemName:      newPrice.ItemName,
				InitialPrice:  newPrice.InitialPrice,
				CurrentPrice:  newPrice.CurrentPrice,
				Reason:        newPrice.Reason,
				MarketID:      newPrice.MarketID,
				CategoryID:    newPrice.CategoryID,
				ChangePercent: newPrice.ChangePercent,
				CreatedAt:     time.Now(),
			}
			if err := tx.Create(&history).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to create price history for %s: %v", barang.Nama, err)})
			}
		}
	}

	// Sync from price to barang
	for _, price := range priceItems {
		if barang, exists := barangMap[price.ItemName]; exists {
			// If barang exists but values are different, update barang
			if barang.HargaSekarang != price.CurrentPrice {
				// Create barang history before updating
				history := models.BarangHistory{
					BarangID:       barang.IdBarang,
					HargaPedagang1: barang.HargaPedagang1,
					HargaPedagang2: barang.HargaPedagang2,
					HargaPedagang3: barang.HargaPedagang3,
					HargaSekarang:  barang.HargaSekarang,
					TanggalUpdate:  time.Now(),
				}
				if err := tx.Create(&history).Error; err != nil {
					tx.Rollback()
					return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to create barang history for %s: %v", price.ItemName, err)})
				}

				// Update barang
				barang.HargaSebelumnya = barang.HargaSekarang
				barang.HargaSekarang = price.CurrentPrice
				barang.AlasanPerubahan = "Synchronized from web app"
				barang.TanggalUpdate = time.Now()

				if err := tx.Save(&barang).Error; err != nil {
					tx.Rollback()
					return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to update barang for %s: %v", price.ItemName, err)})
				}
			}
		} else {
			// If barang doesn't exist, create a new barang entry
			// For new barang, we need to set default values for the three merchant prices
			// We'll set them all to the current price for simplicity
			avgPrice := price.CurrentPrice

			newBarang := models.Barang{
				Nama:            price.ItemName,
				Satuan:          "unit", // Default value
				HargaPedagang1:  avgPrice,
				HargaPedagang2:  avgPrice,
				HargaPedagang3:  avgPrice,
				HargaSebelumnya: price.InitialPrice,
				HargaSekarang:   price.CurrentPrice,
				AlasanPerubahan: "Created from web app data",
				TanggalUpdate:   time.Now(),
			}

			// Set category if available
			if price.CategoryID > 0 {
				categoryID := uint(price.CategoryID)
				newBarang.CategoryID = &categoryID
			}

			if err := tx.Create(&newBarang).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to create barang for %s: %v", price.ItemName, err)})
			}
		}
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to commit transaction: %v", err)})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Synchronization completed successfully",
	})
}

// SyncBarangWithPrice synchronizes a single barang with price
func SyncBarangWithPrice(barangID uint64, tx *gorm.DB) error {
	var barang models.Barang
	if err := tx.First(&barang, barangID).Error; err != nil {
		return fmt.Errorf("failed to find barang: %v", err)
	}

	var price models.Price
	if err := tx.Where("item_name = ?", barang.Nama).First(&price).Error; err != nil {
		// Price doesn't exist, create a new one
		var marketID, categoryID uint
		marketID = barang.MarketID // Langsung ambil dari field barang

		if barang.CategoryID != nil {
			categoryID = uint(*barang.CategoryID)
		} else {
			categoryID = 1 // Default jika kosong
		}

		newPrice := models.Price{
			ItemID:       uint(barang.IdBarang),
			ItemName:     barang.Nama,
			InitialPrice: barang.HargaSebelumnya,
			CurrentPrice: barang.HargaSekarang,
			Reason:       barang.AlasanPerubahan,
			MarketID:     marketID,
			CategoryID:   categoryID,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}

		// Hitung persentase perubahan dengan aman (hindari pembagian dengan nol)
		if barang.HargaSebelumnya > 0 {
			newPrice.ChangePercent = ((barang.HargaSekarang - barang.HargaSebelumnya) / barang.HargaSebelumnya) * 100
		} else {
			// Untuk barang baru atau harga sebelumnya 0, set persentase perubahan ke 0 atau nilai lain yang sesuai
			newPrice.ChangePercent = 0 // atau 100 jika ingin menandai sebagai barang baru
		}

		if err := tx.Create(&newPrice).Error; err != nil {
			return fmt.Errorf("failed to create price: %v", err)
		}

		// Create price history
		history := models.PriceHistory{
			ItemID:        newPrice.ItemID,
			ItemName:      newPrice.ItemName,
			InitialPrice:  newPrice.InitialPrice,
			CurrentPrice:  newPrice.CurrentPrice,
			Reason:        newPrice.Reason,
			MarketID:      newPrice.MarketID,
			CategoryID:    newPrice.CategoryID,
			ChangePercent: newPrice.ChangePercent,
			CreatedAt:     time.Now(),
		}
		if err := tx.Create(&history).Error; err != nil {
			return fmt.Errorf("failed to create price history: %v", err)
		}
	} else {
		// Price exists, update it if needed
		if price.CurrentPrice != barang.HargaSekarang {
			price.InitialPrice = price.CurrentPrice
			price.CurrentPrice = barang.HargaSekarang
			if price.InitialPrice > 0 {
				price.ChangePercent = ((price.CurrentPrice - price.InitialPrice) / price.InitialPrice) * 100
			} else {
				price.ChangePercent = 0 // atau nilai lain yang sesuai
			}
			price.Reason = barang.AlasanPerubahan
			price.UpdatedAt = time.Now()

			if err := tx.Save(&price).Error; err != nil {
				return fmt.Errorf("failed to update price: %v", err)
			}

			// Create price history
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
				return fmt.Errorf("failed to create price history: %v", err)
			}
		}
	}

	return nil
}

// SyncPriceWithBarang synchronizes a single price with barang
func SyncPriceWithBarang(priceID uint, tx *gorm.DB) error {
	var price models.Price
	if err := tx.First(&price, priceID).Error; err != nil {
		return fmt.Errorf("failed to find price: %v", err)
	}

	var barang models.Barang
	if err := tx.Where("nama = ?", price.ItemName).First(&barang).Error; err != nil {
		// Barang doesn't exist, create a new one
		avgPrice := price.CurrentPrice

		// ✅ Validasi MarketID
		marketID := price.MarketID
		var market models.Market
		if err := tx.First(&market, marketID).Error; err != nil {
			// marketID tidak valid → fallback ke default 1
			marketID = 1
		}

		newBarang := models.Barang{
			Nama:            price.ItemName,
			Satuan:          "unit",
			HargaPedagang1:  avgPrice,
			HargaPedagang2:  avgPrice,
			HargaPedagang3:  avgPrice,
			HargaSebelumnya: price.InitialPrice,
			HargaSekarang:   price.CurrentPrice,
			AlasanPerubahan: price.Reason,
			MarketID:        marketID, // ⬅️ penting!
			TanggalUpdate:   time.Now(),
		}

		if price.CategoryID > 0 {
			categoryID := uint(price.CategoryID)
			newBarang.CategoryID = &categoryID
		}

		if err := tx.Create(&newBarang).Error; err != nil {
			return fmt.Errorf("failed to create barang: %v", err)
		}
	} else {
		// Barang exists, update it if needed
		if barang.HargaSekarang != price.CurrentPrice {
			// Simpan histori sebelum update
			history := models.BarangHistory{
				BarangID:       barang.IdBarang,
				HargaPedagang1: barang.HargaPedagang1,
				HargaPedagang2: barang.HargaPedagang2,
				HargaPedagang3: barang.HargaPedagang3,
				HargaSekarang:  barang.HargaSekarang,
				TanggalUpdate:  time.Now(),
			}
			if err := tx.Create(&history).Error; err != nil {
				return fmt.Errorf("failed to create barang history: %v", err)
			}

			// Lanjut update barang
			barang.HargaSebelumnya = barang.HargaSekarang
			barang.HargaSekarang = price.CurrentPrice
			barang.AlasanPerubahan = price.Reason
			barang.TanggalUpdate = time.Now()

			if err := tx.Save(&barang).Error; err != nil {
				return fmt.Errorf("failed to update barang: %v", err)
			}
		}
	}

	return nil
}
