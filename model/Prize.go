package model

import (
	"telebot/database"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Prize struct {
	gorm.Model
	Name   string `gorm:"size:255;not null;" json:"name"`
	ChatId int64
	Date   datatypes.Date
}

func GetPrizeByDate(date datatypes.Date, chatId int64) (*Prize, error) {
	var result Prize
	err := database.Database.Model(Prize{}).Where("date = ? AND chat_id = ?", date, chatId).First(&result).Error

	return &result, err
}
