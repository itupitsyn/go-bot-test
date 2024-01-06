package model

import (
	"telebot/database"
)

type Admin struct {
	ChatID int64  `gorm:"primaryKey"`
	UserID *int64 `gorm:"primaryKey"`
}

func (admin *Admin) Save() (*Admin, error) {
	err := database.Database.Save(&admin).Error

	if err != nil {
		return &Admin{}, err
	}

	return admin, nil
}

func (admin *Admin) IsAdmin() (bool, error) {
	err := database.Database.First(admin).Error

	return err == nil, err
}
