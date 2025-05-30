package models

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

type Barang struct {
	IdBarang        uint64         `gorm:"primaryKey;autoIncrement;column:id_barang" json:"id_barang"`
	Nama            string         `json:"nama"`
	Satuan          string         `json:"satuan"`
	HargaPedagang1  float64        `json:"harga_pedagang1"`
	HargaPedagang2  float64        `json:"harga_pedagang2"`
	HargaPedagang3  float64        `json:"harga_pedagang3"`
	HargaSebelumnya float64        `json:"harga_sebelumnya"`
	HargaSekarang   float64        `json:"harga_sekarang"`
	AlasanPerubahan string         `json:"alasan_perubahan"`
	CategoryID      *uint          `json:"category_id"`
	MarketID        uint           `json:"market_id"`
	Category        Category       `gorm:"foreignKey:CategoryID" json:"category"`
	TanggalUpdate   time.Time      `gorm:"autoUpdateTime" json:"tanggal_update"` // Add this field
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// func MigrateBarang(db *gorm.DB) {
// 	// 🔥 Drop dulu kalau ada kesalahan struktur sebelumnya
// 	_ = db.Migrator().DropTable(&Barang{})

// 	err := db.AutoMigrate(&Barang{})
// 	if err != nil {
// 		panic("❌ Gagal migrasi tabel Barang: " + err.Error())
// 	}
// }

func MigrateBarang(db *gorm.DB) {
	// Check if table exists first to avoid dropping existing data
	if !db.Migrator().HasTable(&Barang{}) {
		err := db.Migrator().CreateTable(&Barang{})
		if err != nil {
			panic("❌ Gagal membuat tabel Barang: " + err.Error())
		}
	}

	// Add columns if they don't exist
	if !db.Migrator().HasColumn(&Barang{}, "tanggal_update") {
		err := db.Migrator().AddColumn(&Barang{}, "tanggal_update")
		if err != nil {
			panic("❌ Gagal menambahkan kolom tanggal_update: " + err.Error())
		}
	}

	fmt.Println("✅ Tabel Barang sudah dimigrasi dengan sukses!")
}
