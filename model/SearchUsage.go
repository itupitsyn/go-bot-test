package model

import (
	"fmt"
	"time"
)

// SearchUsage counts requests to a paid search engine per calendar month, so a
// monthly budget survives restarts. Brave bills per request: $5 of free credit
// a month plus what the owner agreed to pay on top ($5, 06.11.2026), 2000
// requests in all, with a margin kept below that by the caller.
type SearchUsage struct {
	Engine    string `gorm:"primaryKey;size:32"`
	Month     string `gorm:"primaryKey;size:7"`
	Count     int    `gorm:"not null;default:0"`
	UpdatedAt time.Time
}

// searchMonth is the billing month the request counts toward, in UTC.
func searchMonth(now time.Time) string {
	return now.UTC().Format("2006-01")
}

// ReserveSearchQuota takes one request out of the engine's monthly budget.
// True means the request may go; false means the month's limit is reached.
//
// One statement, so two checks at the same moment cannot both squeeze past the
// limit: the row is created at 1 for a new month, and otherwise grows by one
// only while it is below the limit. A refused update touches no rows.
func ReserveSearchQuota(engine string, limit int, now time.Time) (bool, error) {
	if limit <= 0 {
		return false, nil
	}

	result := db.Exec(`INSERT INTO search_usages (engine, month, count, updated_at) VALUES (?, ?, 1, ?)
ON CONFLICT (engine, month) DO UPDATE SET count = search_usages.count + 1, updated_at = EXCLUDED.updated_at
WHERE search_usages.count < ?`, engine, searchMonth(now), now, limit)
	if result.Error != nil {
		return false, fmt.Errorf("reserve %s search quota: %w", engine, result.Error)
	}

	return result.RowsAffected == 1, nil
}
