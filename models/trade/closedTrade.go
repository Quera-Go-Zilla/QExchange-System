package trade

import "gorm.io/gorm"

// closed trade should change a lot
type ClosedTrade struct {
	gorm.Model
	UserID      uint    `gorm:"not null" json:"user_id"`
	CryptoID    uint    `gorm:"not null" json:"crypto_id"`
	Amount      float64 `gorm:"not null" json:"amount"`
	BuyFee		int     `gorm:"not null" json:"buy_fee"`
	SellFee		int 	`gorm:"not null" json:"sell_fee"`
}

func (ClosedTrade) TableName() string {
	return "closed_trade"
}