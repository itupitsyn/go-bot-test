package model

import (
	"telebot/database"
	"time"
)

type User struct {
	ID        int64  `gorm:"primaryKey"`
	Name      string `gorm:"size:255;not null;unique" json:"name"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt time.Time
	Raffle    []Raffle `gorm:"foreignKey:WinnerID;"`
}

func (user *User) Save() (*User, error) {
	err := database.Database.Save(&user).Error
	if err != nil {
		return &User{}, err
	}
	return user, nil
}
