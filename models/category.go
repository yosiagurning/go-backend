package models

import (
	"fmt"
	"log"

	"gorm.io/gorm"
)

type Category struct {
	ID          uint     `json:"id" gorm:"primaryKey"`
	Name        string   `json:"name" gorm:"not null"`
	Description string   `json:"description"`
	Markets     []Market `json:"markets" gorm:"many2many:category_markets"`
	Prices      []Price  `json:"prices" gorm:"foreignKey:CategoryID"` // Tambahkan relasi ke Price
	Barangs     []Barang `gorm:"foreignKey:CategoryID" json:"barangs"`
}

// Fungsi untuk migrasi Category
func MigrateCategory(db *gorm.DB) {
	if db.Migrator().HasTable(&Category{}) {
		fmt.Println("✅ Tabel Category sudah ada, skip migrasi ulang.")
		return
	}

	if err := db.AutoMigrate(&Category{}); err != nil {
		log.Fatalf("❌ Gagal migrasi tabel Category: %v", err)
	}

	fmt.Println("✅ Migrasi Category berhasil.")
}
