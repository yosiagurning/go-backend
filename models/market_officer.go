package models

import (
	"time"

	"gorm.io/gorm"
)

type MarketOfficer struct {
	ID        uint64    `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name"`
	Nik       string    `json:"nik" gorm:"type:varchar(255);uniqueIndex:idx_market_officers_nik"`
	Phone     string    `json:"phone"`
	ImageURL  string    `json:"image_url"`
	Username  string    `json:"username" gorm:"type:varchar(255);uniqueIndex:idx_market_officers_username"`
	Password  string    `json:"-"`
	MarketID  uint64    `json:"market_id"`
	Market    Market    `json:"market" gorm:"foreignKey:MarketID;references:ID"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	IsActive  bool      `json:"is_active" gorm:"default:true"`
}
type MarketOfficerResponse struct {
	ID       uint64          `json:"id"`
	Name     string          `json:"name"`
	Username string          `json:"username"`
	Nik      string          `json:"nik"`
	Phone    string          `json:"phone"`
	ImageURL string          `json:"image_url"`
	MarketID uint64          `json:"market_id"`
	Market   *MarketResponse `json:"market"`
}

// MigrateMarketOfficer membuat tabel MarketOfficer jika belum ada
func MigrateMarketOfficer(db *gorm.DB) {
	if err := db.Migrator().DropTable(&MarketOfficer{}); err != nil {
		panic("Failed to drop the existing MarketOfficer table: " + err.Error())
	}

	if err := db.AutoMigrate(&MarketOfficer{}); err != nil {
		panic("Failed to migrate MarketOfficer table: " + err.Error())
	}
}
