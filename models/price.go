package models

import (
	"time"

	"gorm.io/gorm"
)

type Price struct {
	ID            uint           `json:"id" gorm:"primaryKey"`
	ItemID        uint           `json:"item_id"`
	ItemName      string         `json:"item_name"`
	InitialPrice  float64        `json:"initial_price"`
	CurrentPrice  float64        `json:"current_price"`
	ChangePercent float64        `json:"change_percent"`
	Reason        string         `json:"reason"`
	MarketID      uint           `json:"market_id"`
	Market        Market         `json:"market" gorm:"foreignKey:MarketID"`
	CategoryID    uint           `json:"category_id"`
	Category      Category       `json:"category" gorm:"foreignKey:CategoryID"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"` // optional soft delete
}

func MigratePrice(db *gorm.DB) {
	db.AutoMigrate(&Price{})
}
