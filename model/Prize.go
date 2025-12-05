package model

import (
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Prize struct {
	gorm.Model
	Name   string `gorm:"not null;" json:"name"`
	ChatID int64
	Date   datatypes.Date
}

func (prize *Prize) Save() (*Prize, error) {
	err := db.Save(&prize).Error
	if err != nil {
		return &Prize{}, err
	}
	return prize, nil
}

func GetPrizeByDate(date datatypes.Date, chatId int64) (*Prize, error) {
	var result Prize
	err := db.Model(Prize{}).Where("date = ? AND chat_id = ?", date, chatId).First(&result).Error

	return &result, err
}

func GetPrizesByDate(dates []datatypes.Date, chatId int64) (*[]Prize, error) {
	var result []Prize
	err := db.Model(Prize{}).Where("date in (?) AND chat_id = ?", dates, chatId).Find(&result).Error

	return &result, err
}

func DeletePrizeByDate(date datatypes.Date, chatId int64) error {
	err := db.Model(Prize{}).Where("date = ? AND chat_id = ?", date, chatId).Unscoped().Delete(&Prize{}).Error
	return err
}
