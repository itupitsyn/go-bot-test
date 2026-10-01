package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// aiLimitSettingsID: the general refusal wording is one for the whole bot, so
// its table holds one row, same as ai_maintenances.
const aiLimitSettingsID = 1

// Periods a limit can be counted over. The window SLIDES: "20 per hour" means
// twenty in the last sixty minutes, not twenty since the clock struck. A fixed
// window would let a person spend the whole quota at 10:59 and the next one at
// 11:00, which is exactly the burst the limit exists to stop.
const (
	AiPeriodHour = "hour"
	AiPeriodDay  = "day"
	AiPeriodWeek = "week"
)

// AiLimitWindow is how far back the counting goes for a period. An unknown
// period gives zero, and the caller reads that as "no limit": a typo in the
// database must not lock people out of the bot.
func AiLimitWindow(period string) time.Duration {
	switch period {
	case AiPeriodHour:
		return time.Hour
	case AiPeriodDay:
		return 24 * time.Hour
	case AiPeriodWeek:
		return 7 * 24 * time.Hour
	}

	return 0
}

// AiLimit is one person's quota for the heavy AI commands: pictures, edits,
// videos and sticker packs. Transcription and retelling are not counted, they
// cost almost nothing and limiting them would get in the way of ordinary
// chatting.
//
// No row means no limit, and that is the normal state for almost everybody.
// The admin panel adds a row for the few who need one.
type AiLimit struct {
	UserID int64 `gorm:"primaryKey" json:"user_id"`
	// Amount is how many generations fit in the window. Zero or less means no
	// limit — the admin panel empties the field instead of deleting the row.
	Amount int    `gorm:"not null;default:0" json:"amount"`
	Period string `gorm:"size:16;not null;default:'hour'" json:"period"`
	// Message is this person's own refusal wording. Empty means the general
	// one is used.
	Message   string `gorm:"not null;default:''" json:"message"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsActive says whether this limit restricts anything at all.
func (l *AiLimit) IsActive() bool {
	return l != nil && l.Amount > 0 && AiLimitWindow(l.Period) > 0
}

// AiLimitSettings holds what is common to all limits — for now the general
// refusal wording.
type AiLimitSettings struct {
	ID        uint   `gorm:"primaryKey"`
	Message   string `gorm:"not null;default:''" json:"message"`
	UpdatedAt time.Time
}

// AiUsage is one charge against a quota. One ROW per command, with Units
// saying how much it cost: a sticker pack is ten generations and costs ten,
// while a picture costs one.
//
// Units lives in the row rather than in ten separate rows so that a pack that
// came out short can be corrected afterwards by rewriting one number.
type AiUsage struct {
	ID     uint64 `gorm:"primaryKey"`
	UserID int64  `gorm:"not null;index:idx_ai_usages_user_created,priority:1"`
	// Kind is what was asked for. Nothing depends on it; it is there so that
	// the table can be looked at later and understood.
	Kind      string    `gorm:"size:16;not null"`
	Units     int       `gorm:"not null;default:1"`
	CreatedAt time.Time `gorm:"not null;index:idx_ai_usages_user_created,priority:2"`
}

// GetAiLimit returns the person's quota, or nil when they have none.
func GetAiLimit(userID int64) (*AiLimit, error) {
	var limit AiLimit
	err := db.Where("user_id = ?", userID).First(&limit).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &limit, nil
}

// GetAiLimitSettings returns the common settings. Until the admin panel has
// saved them even once there is no row, and that means "no wording of our
// own", not an error.
func GetAiLimitSettings() (*AiLimitSettings, error) {
	var settings AiLimitSettings
	err := db.First(&settings, aiLimitSettingsID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &settings, nil
}

// CountAiUsage adds up what the person has spent since the given moment.
func CountAiUsage(userID int64, since time.Time) (int, error) {
	var total *int
	err := db.Model(&AiUsage{}).
		Where("user_id = ? AND created_at > ?", userID, since).
		Select("SUM(units)").Scan(&total).Error
	if err != nil {
		return 0, err
	}
	if total == nil { // ничего не тратил — SUM по пустому множеству даёт NULL
		return 0, nil
	}

	return *total, nil
}

// AddAiUsage charges the quota and returns the row, so that a sticker pack
// which came out short can correct itself afterwards.
func AddAiUsage(userID int64, kind string, units int) (*AiUsage, error) {
	usage := AiUsage{UserID: userID, Kind: kind, Units: units}
	if err := db.Create(&usage).Error; err != nil {
		return nil, err
	}

	return &usage, nil
}

// SetUnits rewrites what the charge ended up costing.
func (u *AiUsage) SetUnits(units int) error {
	if u == nil {
		return nil
	}
	if units <= 0 {
		return db.Delete(u).Error
	}

	return db.Model(u).Update("units", units).Error
}

// SaveAiLimit writes a person's quota, overwriting whatever was there.
func SaveAiLimit(limit *AiLimit) error {
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"amount", "period", "message", "updated_at"}),
	}).Create(limit).Error
}

// DeleteAiLimit removes a person's quota. Missing is not an error: the caller
// wants it gone, and gone it is.
func DeleteAiLimit(userID int64) error {
	return db.Where("user_id = ?", userID).Delete(&AiLimit{}).Error
}

// PruneAiUsage throws away charges older than the longest window. Without it
// the table grows forever, and nothing older than a week can affect a decision.
func PruneAiUsage(now time.Time) (int64, error) {
	cutoff := now.Add(-AiLimitWindow(AiPeriodWeek) - time.Hour)
	res := db.Where("created_at < ?", cutoff).Delete(&AiUsage{})

	return res.RowsAffected, res.Error
}
