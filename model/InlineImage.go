package model

import (
	"crypto/rand"
	"encoding/base64"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// inlineImageTokenBytes is the amount of randomness behind a preview URL.
const inlineImageTokenBytes = 32

// InlineImageLimit is how many pictures are kept per user. The buffer works as
// a ring: an upload past the limit pushes out the least recently touched
// picture. The same number is offered in inline results, so raising it makes
// the inline list longer as well.
const InlineImageLimit = 5

// InlineImageTTL is how long a picture nobody touches stays in the buffer.
const InlineImageTTL = 7 * 24 * time.Hour

// InlineImage is a picture the user has sent to the bot in a private chat, kept
// so that it can be offered for animation in inline mode. Only the Telegram
// file id is stored, never the picture itself: the file lives on Telegram's
// side and a record here is under a hundred bytes.
//
// The model deliberately has no gorm.Model embedded: soft deletes would keep
// the rows the rotation is supposed to get rid of.
type InlineImage struct {
	ID     uint   `gorm:"primaryKey"`
	UserID int64  `gorm:"not null;index:idx_inline_images_user_updated,priority:1;uniqueIndex:idx_inline_images_user_file,priority:1"`
	FileID string `gorm:"not null"`
	// ThumbFileID points at the smallest size Telegram offers for the picture
	// and is what the preview endpoint serves. Rows written before previews
	// existed have it empty and fall back to FileID.
	ThumbFileID string `gorm:"not null;default:''"`
	// Token is what the preview URL of the picture is addressed by. It is random
	// enough that it cannot be guessed, and it never changes once assigned:
	// Telegram caches previews by URL, so a moving address would leave holes in
	// the inline list. The index is deliberately not unique — 32 random bytes
	// will not collide, and a unique index cannot be built while rows written
	// before this column existed still share an empty value.
	Token        string `gorm:"not null;default:'';index"`
	FileUniqueID string `gorm:"not null;uniqueIndex:idx_inline_images_user_file,priority:2"`
	CreatedAt    time.Time
	UpdatedAt    time.Time `gorm:"index:idx_inline_images_user_updated,priority:2"`
}

// newInlineImageToken mints the secret part of a preview URL.
func newInlineImageToken() (string, error) {
	buf := make([]byte, inlineImageTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// SaveInlineImage puts a picture into the user's buffer. Sending a picture that
// is already there refreshes it instead of creating a duplicate, so it moves
// back to the top; file_unique_id is stable for the same file, unlike file_id.
// Everything past the InlineImageLimit most recently touched pictures is
// dropped in the same transaction.
func SaveInlineImage(userID int64, fileID string, thumbFileID string, fileUniqueID string) error {
	token, err := newInlineImageToken()
	if err != nil {
		return err
	}

	image := InlineImage{
		UserID:       userID,
		FileID:       fileID,
		ThumbFileID:  thumbFileID,
		Token:        token,
		FileUniqueID: fileUniqueID,
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// The token is missing from DoUpdates on purpose: re-sending a picture
		// refreshes it but keeps the preview URL it already has.
		err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "file_unique_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"file_id", "thumb_file_id", "updated_at"}),
		}).Create(&image).Error
		if err != nil {
			return err
		}

		return trimInlineImages(tx, userID)
	})
}

// trimInlineImages keeps the InlineImageLimit most recently touched pictures of
// the user and deletes the rest. The id tie-breaker makes the cut deterministic
// when several pictures share an updated_at value.
func trimInlineImages(tx *gorm.DB, userID int64) error {
	return tx.Exec(`
		DELETE FROM inline_images
		WHERE user_id = ? AND id NOT IN (
			SELECT id FROM inline_images
			WHERE user_id = ?
			ORDER BY updated_at DESC, id DESC
			LIMIT ?
		)`, userID, userID, InlineImageLimit).Error
}

// GetInlineImages returns the user's pictures, most recently touched first.
func GetInlineImages(userID int64, limit int) ([]InlineImage, error) {
	var images []InlineImage
	err := db.
		Where("user_id = ?", userID).
		Order("updated_at DESC, id DESC").
		Limit(limit).
		Find(&images).Error

	return images, err
}

// GetInlineImageByToken returns the picture a preview URL points at. An empty
// token matches nothing, so that a bare request to the endpoint cannot pick up
// a row that has not been given a token yet.
func GetInlineImageByToken(token string) (*InlineImage, error) {
	if token == "" {
		return nil, gorm.ErrRecordNotFound
	}

	var image InlineImage
	err := db.Where("token = ?", token).First(&image).Error

	return &image, err
}

// BackfillInlineImageTokens gives a preview URL to the pictures saved before
// this column existed and reports how many it touched. Without it those rows
// would sit in the buffer with no preview until the TTL takes them.
func BackfillInlineImageTokens() (int, error) {
	var images []InlineImage
	if err := db.Where("token = ''").Find(&images).Error; err != nil {
		return 0, err
	}

	for _, image := range images {
		token, err := newInlineImageToken()
		if err != nil {
			return 0, err
		}

		if err := db.Model(&InlineImage{}).Where("id = ?", image.ID).Update("token", token).Error; err != nil {
			return 0, err
		}
	}

	return len(images), nil
}

// DeleteInlineImage drops a single picture of the user. It is called when
// Telegram no longer serves the file id, so that a dead picture stops showing
// up in inline results.
func DeleteInlineImage(userID int64, fileID string) error {
	return db.Where("user_id = ? AND file_id = ?", userID, fileID).Delete(&InlineImage{}).Error
}

// DeleteInlineImagesByUser empties the user's buffer and reports how many
// pictures were dropped.
func DeleteInlineImagesByUser(userID int64) (int64, error) {
	res := db.Where("user_id = ?", userID).Delete(&InlineImage{})
	return res.RowsAffected, res.Error
}

// DeleteOldInlineImages drops pictures untouched for longer than InlineImageTTL
// and reports how many were removed. Rotation caps the buffer of an active
// user; this is what collects after the ones who tried the bot once and left.
func DeleteOldInlineImages() (int64, error) {
	res := db.Where("updated_at < ?", time.Now().UTC().Add(-InlineImageTTL)).Delete(&InlineImage{})
	return res.RowsAffected, res.Error
}
