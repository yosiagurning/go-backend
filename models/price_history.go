package models

import (
	"time"
)

type PriceHistory struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	ItemID       uint      `json:"item_id"`
	ItemName     string    `json:"item_name"`
	InitialPrice float64   `json:"initial_price"`
	CurrentPrice float64   `json:"current_price"`
	Reason       string    `json:"reason"`
	MarketID     uint      `json:"market_id"`
	CategoryID   uint      `json:"category_id"`
	ChangePercent float64  `json:"change_percent"`
	CreatedAt    time.Time `json:"created_at"`
}
