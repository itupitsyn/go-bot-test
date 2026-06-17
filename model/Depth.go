package model

import (
	"time"

	"gorm.io/datatypes"
)

type Depth struct {
	UserID    int64          `gorm:"primaryKey"`
	Date      datatypes.Date `gorm:"primaryKey"`
	Value     int            `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// GetOrCreateTodayDepth returns the depth fixed for the user on the current day.
// On the first request of the day it persists the provided generated value;
// subsequent requests return the stored one. The day key is derived from the
// current UTC date, so a new day has no record and the depth is regenerated at
// 00:00 UTC, regardless of the host's local timezone.
func GetOrCreateTodayDepth(userID int64, generated int) (int, error) {
	today := datatypes.Date(time.Now().UTC())

	depth := Depth{UserID: userID, Date: today}
	err := db.
		Where(Depth{UserID: userID, Date: today}).
		Attrs(Depth{Value: generated}).
		FirstOrCreate(&depth).Error

	return depth.Value, err
}
