package model

import (
	"telebot/database"
)

type ChatUserRole struct {
	UserID int64 `gorm:"primaryKey"`
	ChatID int64 `gorm:"primaryKey"`
	RoleID int64
}

func (chatUserRole *ChatUserRole) Save() (*ChatUserRole, error) {
	err := database.Database.Save(&chatUserRole).Error
	if err != nil {
		return &ChatUserRole{}, err
	}
	return chatUserRole, nil
}
