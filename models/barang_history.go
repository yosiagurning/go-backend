package models

import (
	"time"

	"gorm.io/gorm"
)

type BarangHistory struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	BarangID       uint64    `json:"barang_id"`
	Barang         Barang    `gorm:"foreignKey:BarangID;constraint:OnDelete:CASCADE"`
	HargaPedagang1 float64   `json:"harga_pedagang1"`
	HargaPedagang2 float64   `json:"harga_pedagang2"`
	HargaPedagang3 float64   `json:"harga_pedagang3"`
	HargaSekarang  float64   `json:"harga_sekarang"`
	TanggalUpdate  time.Time `json:"tanggal_update"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func MigrateBarangHistory(db *gorm.DB) {
	if !db.Migrator().HasTable(&BarangHistory{}) {
		err := db.Migrator().CreateTable(&BarangHistory{})
		if err != nil {
			panic("❌ Gagal membuat tabel BarangHistory: " + err.Error())
		}
	}
}
