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

type stats struct {
	Name            string
	Alternativename string
	Count           int32
}

func GetStats(chatId int64) *[]stats {
	var result []stats
	subQuery := database.Database.Model(&Raffle{}).Select("count (*) as Count, winner_id").Where("winner_id is not null and chat_id = ?", chatId).Group("winner_id")
	database.Database.Model(&User{}).Select("users.name as Name, users.alternative_name as Alternativename, kk.Count").Joins("inner join (?) as kk on users.id = kk.winner_id", subQuery).Order("Count desc").Find(&result)
	return &result
}
