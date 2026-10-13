package model

type Phraze struct {
	Key           string `gorm:"primaryKey;size:255;not null;"`
	Value         string `gorm:"primaryKey;not null;"`
	IsUncensored  bool   `json:"is_uncensored"`
	IsWithSpoiler bool   `gorm:"default:false" json:"is_with_spoiler"`
	Group         *int
	Order         *int
}

// Order() must come BEFORE Find(). Find is a finisher: it runs the query, and
// anything chained after it never reaches the SQL. Written the other way round
// these two silently returned rows in whatever order Postgres felt like, which
// matters because GetRandomPhrazeByKey appends a whole group in the order it
// receives it and does not sort by Order itself.
func getAnyPharzesByKey(key string) (*[]Phraze, error) {
	var result []Phraze
	err := db.Model(&Phraze{}).Where("key = ?", key).Order("\"group\" ASC, \"order\" ASC").Find(&result).Error

	return &result, err
}

// The flag means two different things on the two tables, and they are easy to
// confuse: chats.is_uncensored says the CHAT is allowed rude phrases, while
// phrazes.is_uncensored says the PHRASE itself is rude. This function serves
// the chats where rude phrases are NOT allowed, so it must take the polite
// ones — is_uncensored = false. It asked for true until 13.10.2026, which
// handed censored chats exactly the phrases they are meant to be spared.
func getCensoredPharzesByKey(key string) (*[]Phraze, error) {
	var result []Phraze
	err := db.Model(&Phraze{}).Where("key = ? and is_uncensored = ?", key, false).Order("\"group\" ASC, \"order\" ASC").Find(&result).Error

	return &result, err
}

func GetPharzesByKey(key string, isUncensored bool) (*[]Phraze, error) {
	if isUncensored {
		return getAnyPharzesByKey(key)
	}

	return getCensoredPharzesByKey(key)
}
