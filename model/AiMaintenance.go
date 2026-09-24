package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// aiMaintenanceID: there is one maintenance setting for the whole bot, so there
// is one row in the table too. The admin panel creates and edits it.
const aiMaintenanceID = 1

// AiMaintenance is maintenance of the AI features: while it is on, the bot does
// not go to the generation services and instead answers that it is under repair
// and when it will be back.
type AiMaintenance struct {
	ID        uint `gorm:"primaryKey"`
	IsEnabled bool `gorm:"not null;default:false"`
	// EndsAt is when we promise to be back. Empty means no deadline was named.
	EndsAt    *time.Time
	UpdatedAt time.Time
}

// IsActive says whether maintenance is on at moment now. A named deadline ends
// it by itself: there is no need to switch it off by hand when everything got
// fixed on time.
func (m *AiMaintenance) IsActive(now time.Time) bool {
	if m == nil || !m.IsEnabled {
		return false
	}

	return m.EndsAt == nil || now.Before(*m.EndsAt)
}

// GetAiMaintenance returns the maintenance setting. Until the admin panel has
// saved it even once there is no row, which means "no maintenance", not an
// error.
func GetAiMaintenance() (*AiMaintenance, error) {
	var maintenance AiMaintenance
	err := db.First(&maintenance, aiMaintenanceID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &maintenance, nil
}
