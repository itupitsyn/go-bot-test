package model

import (
	"time"

	"gorm.io/datatypes"
)

type Size struct {
	UserID    int64          `gorm:"primaryKey"`
	Date      datatypes.Date `gorm:"primaryKey"`
	Value     int            `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// GetOrCreateTodaySize returns the size fixed for the user on the current day.
// On the first request of the day it persists the provided generated value;
// subsequent requests return the stored one. A new day (server local time)
// has no record, so the size is regenerated — that is the 00:00 reset.
func GetOrCreateTodaySize(userID int64, generated int) (int, error) {
	today := datatypes.Date(time.Now())

	size := Size{UserID: userID, Date: today}
	err := db.
		Where(Size{UserID: userID, Date: today}).
		Attrs(Size{Value: generated}).
		FirstOrCreate(&size).Error

	return size.Value, err
}
