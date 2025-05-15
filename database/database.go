package database

import (
	"backend/models"
	"fmt"
	"log"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// DB adalah instance global untuk database
var DB *gorm.DB

// Fungsi untuk menghubungkan ke database
func ConnectDatabase() {
	var err error
	dsn := "root:OEEYcvQBItzCMavRHbNQyWlFkrHXwBxU@tcp(shinkansen.proxy.rlwy.net:25817)/railway?charset=utf8mb4&parseTime=True&loc=Local"
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	fmt.Println("✅ Database connected successfully!")

	// Migrasi model ke dalam database
	err = DB.AutoMigrate(&models.Price{}, &models.Market{}, &models.User{}, &models.Barang{}, &models.BarangHistory{}, &models.PriceHistory{}, &models.Category{}, &models.MarketOfficer{},&models.CategoryMarket{})
	if err != nil {
		log.Fatalf("❌ Failed to migrate the database: %v\n", err)
	}
	fmt.Println("✅ Database migrated successfully!")
}
