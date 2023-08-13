package model

import (
	"telebot/database"
	"time"

	"gorm.io/datatypes"
)

type Raffle struct {
	Date         datatypes.Date `gorm:"primaryKey"`
	ChatID       int64          `gorm:"primaryKey"`
	Participants []User         `gorm:"many2many:participants;"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAtAt  time.Time
	WinnerID     *int64
}

func (raffle *Raffle) Save() (*Raffle, error) {

	err := database.Database.Save(&raffle).Error

	if err != nil {
		return &Raffle{}, err
	}

	return raffle, nil
}

func (raffle *Raffle) GetRafflesByDate(date datatypes.Date) ([]Raffle, error) {
	var raffles []Raffle
	err := database.Database.Model(&Raffle{}).Preload("Participants").Where("Date = ?", date).Find(&raffles).Error

	return raffles, err
}
