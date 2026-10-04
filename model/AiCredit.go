package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AiCredit is how many generations a person has bought and not yet spent.
//
// Bought units are a SEPARATE pocket from the free quota, not an addition to
// it. The quota is a sliding window over ai_usages — raising it would hand the
// person a bigger burst for ever, and lowering it back is impossible once the
// window has moved. A pocket spends only when the window is already full, and
// what is left in it keeps for next time.
//
// They do not expire. A pack bought a minute before the window rolls over
// would otherwise burn, and that is the first complaint anyone would write.
// Hoarding them costs us nothing: the cap on jobs in flight is what protects
// the cards, and that cap is untouched by any balance.
//
// No row means no bought units, which is the normal state for almost everybody.
type AiCredit struct {
	UserID int64 `gorm:"primaryKey"`
	// Balance is in generations, not in stars: the price may change, and a
	// balance in money would have to be re-divided by a new price every time.
	Balance   int `gorm:"not null;default:0"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AiPurchase is one paid-for pack, kept by the id Telegram gives the payment.
//
// It is here for two reasons. Telegram may deliver the same update twice, and
// the primary key makes a repeat harmless — crediting one payment twice is
// still a bug even when it favours the person. And a refund through
// refundStarPayment needs exactly this id, which nothing else in our tables
// holds.
type AiPurchase struct {
	ChargeID  string `gorm:"primaryKey;size:255"`
	UserID    int64  `gorm:"index;not null"`
	Stars     int    `gorm:"not null"`
	Units     int    `gorm:"not null"`
	CreatedAt time.Time
}

// GetAiCredit returns the person's pocket, or nil when they have never bought
// anything. Missing is a normal answer here, not a failure.
func GetAiCredit(userID int64) (*AiCredit, error) {
	var credit AiCredit
	err := db.Where("user_id = ?", userID).First(&credit).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &credit, nil
}

// AddAiCredit puts units into the pocket, creating it if there is none.
//
// The sum is counted by the database (balance + ?), not read into Go and
// written back: two payments landing together would otherwise both read the
// old number and one of them would vanish.
func AddAiCredit(userID int64, units int) error {
	if units <= 0 {
		return nil
	}

	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"balance":    gorm.Expr("ai_credits.balance + ?", units),
			"updated_at": time.Now(),
		}),
	}).Create(&AiCredit{UserID: userID, Balance: units}).Error
}

// SpendAiCredit takes units out and says whether there was enough.
//
// The check and the subtraction are ONE statement on purpose: asking the
// balance first and subtracting after would let two commands sent at the same
// moment both see the same ten units and both spend them. Here the database
// decides, and a row that no longer satisfies "balance >= units" is simply not
// updated.
func SpendAiCredit(userID int64, units int) (bool, error) {
	if units <= 0 {
		return true, nil
	}

	res := db.Model(&AiCredit{}).
		Where("user_id = ? AND balance >= ?", userID, units).
		Updates(map[string]any{
			"balance":    gorm.Expr("balance - ?", units),
			"updated_at": time.Now(),
		})
	if res.Error != nil {
		return false, res.Error
	}

	return res.RowsAffected > 0, nil
}

// CreditAiPurchase records a payment and puts the units in the pocket, once.
//
// Returns false when this payment has already been counted: Telegram repeats
// an update it thinks went unanswered, and without this the same five stars
// would buy twenty generations as many times as the repeat happens.
//
// Both writes go in one transaction, so a crash between them cannot leave a
// payment recorded but not granted, or granted but not recorded.
func CreditAiPurchase(chargeID string, userID int64, stars, units int) (bool, error) {
	fresh := false
	err := db.Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&AiPurchase{
			ChargeID: chargeID, UserID: userID, Stars: stars, Units: units,
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}

		fresh = true

		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"balance":    gorm.Expr("ai_credits.balance + ?", units),
				"updated_at": time.Now(),
			}),
		}).Create(&AiCredit{UserID: userID, Balance: units}).Error
	})

	return fresh, err
}
