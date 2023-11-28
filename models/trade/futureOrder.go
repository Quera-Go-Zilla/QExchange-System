package trade

import (
	"gorm.io/gorm"
)

type FutureOrder struct {
	gorm.Model
	UserID      uint    `gorm:"not null"`
	CryptoID    uint    `gorm:"not null"`
	Amount      int 	`gorm:"not null"`
	TargetPrice int     `gorm:"not null"`
	StopLoss    int     `gorm:""`
	TakeProfit  int     `gorm:""`
}

func (FutureOrder) TableName() string {
	return "future_order"
}

func (fo FutureOrder) ToOpenTradeRequest() OpenTradeRequest {
	return OpenTradeRequest{
		CryptoID:   fo.CryptoID,
		Amount:     fo.Amount,
		StopLoss:   fo.StopLoss,
		TakeProfit: fo.TakeProfit,
	}
}
