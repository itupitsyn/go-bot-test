package model

import (
	"telebot/database"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Prize struct {
	gorm.Model
	Name string `gorm:"size:255;not null;" json:"name"`
	Date datatypes.Date
}

func GetPrizeByDate(date datatypes.Date) (*Prize, error) {
	var result Prize
	err := database.Database.Model(Prize{Date: date}).First(&result).Error

	return &result, err
}
