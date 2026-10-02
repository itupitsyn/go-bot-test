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
	// PhotoFileID is the Telegram file the pack was drawn from.
	//
	// It is kept so that a single sticker can be redrawn later without asking
	// the person for the photo again — they would have to find it in the chat
	// history, and the one they found might not be the same one. A file id
	// issued to our own bot does not expire.
	//
	// Empty for a pack made before this was stored: such a pack cannot be
	// touched one sticker at a time, only remade whole.
	PhotoFileID string `gorm:"not null;default:''"`
	// WithBody is the answer the pack used about naming poses, see
	// aiApi.GenerateStickers.
	//
	// Stored rather than asked again, and that is the point: the question goes
	// to a vision model, and a second asking of the same photo can come back
	// the other way. A replacement drawn under the opposite answer would sit
	// among nine stickers made under this one — zoomed out and with a torso
	// invented, or frozen while the rest move.
	WithBody bool `gorm:"not null;default:false"`
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
func SaveStickerPack(userID int64, name string, count int, photoFileID string,
	withBody bool) error {
	pack := StickerPack{UserID: userID, Name: name, Count: count,
		PhotoFileID: photoFileID, WithBody: withBody}

	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "count",
			"photo_file_id", "with_body", "updated_at"}),
	}).Create(&pack).Error
}

// DeleteStickerPack forgets the person's pack. Missing is not an error: the
// caller wants the row gone, and gone it is.
func DeleteStickerPack(userID int64) error {
	return db.Where("user_id = ?", userID).Delete(&StickerPack{}).Error
}
