package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// StickerPack remembers which Telegram sticker set belongs to which person.
//
// Only the short name is kept, never the pictures: the set lives on Telegram's
// side, and the name is all that is needed to add to it, replace it or send
// the person back to the one they already have.
//
// One pack per person on purpose. A second one would have to be named
// differently, and Telegram never forgets a set — a person who regenerates
// five times would leave five packs behind with no way to tidy them up.
// Regenerating replaces the stickers inside the same set instead, so the link
// the person has already shared keeps working.
type StickerPack struct {
	// UserID is the owner. Telegram ties a set to the user it was created for,
	// and only that user may add to it, so the owner is the primary key.
	UserID int64 `gorm:"primaryKey"`
	// Name is the set's short name, the one that ends with _by_<bot> and goes
	// into the t.me/addstickers/ link.
	Name string `gorm:"not null"`
	// Count is how many stickers ended up in the set. A pack may come out
	// short: a failed emotion is skipped rather than fatal.
	Count int `gorm:"not null;default:0"`
	// CreatedAt is when the pack was first made, UpdatedAt when it was last
	// refilled.
	CreatedAt time.Time
	UpdatedAt time.Time
}

// GetStickerPack returns the person's pack, or nil when they have none.
func GetStickerPack(userID int64) (*StickerPack, error) {
	var pack StickerPack
	err := db.Where("user_id = ?", userID).First(&pack).Error
	if err != nil {
		// Missing is a normal answer here, not a failure: most people have no
		// pack yet.
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &pack, nil
}

// SaveStickerPack writes the pack down, overwriting whatever the person had.
func SaveStickerPack(userID int64, name string, count int) error {
	pack := StickerPack{UserID: userID, Name: name, Count: count}

	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "count", "updated_at"}),
	}).Create(&pack).Error
}

// DeleteStickerPack forgets the person's pack. Missing is not an error: the
// caller wants the row gone, and gone it is.
func DeleteStickerPack(userID int64) error {
	return db.Where("user_id = ?", userID).Delete(&StickerPack{}).Error
}
