package models

import (
	"log"
	"time"

	"gorm.io/gorm"
)

// Struktur Market dengan timestamps dan soft delete
type Market struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Name      string         `gorm:"not null" json:"name"`
	Location  string         `gorm:"not null" json:"location"`
	ImageURL  string         `json:"image_url"`
	Latitude  float64        `gorm:"default:0" json:"latitude"`
	Longitude float64        `gorm:"default:0" json:"longitude"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
type MarketResponse struct {
	ID        uint    `json:"id"`
	Name      string  `json:"name"`
	Location  string  `json:"location"`
	ImageURL  string  `json:"image_url"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// Fungsi untuk migrasi tabel Market
func MigrateMarket(db *gorm.DB) {
	if db.Migrator().HasTable(&Market{}) {
		log.Println("✅ Market table already exists. Skipping migration.")
		return
	}

	if err := db.AutoMigrate(&Market{}); err != nil {
		log.Fatalf("❌ Failed to migrate Market table: %v", err)
	}

	log.Println("✅ Market table migrated successfully.")
}
