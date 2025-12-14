package model

type Phraze struct {
	Key           string `gorm:"primaryKey;size:255;not null;"`
	Value         string `gorm:"primaryKey;not null;"`
	IsUncensored  bool   `json:"is_uncensored"`
	IsWithSpoiler bool   `gorm:"default:false" json:"is_with_spoiler"`
	Group         *int
	Order         *int
}

func getAnyPharzesByKey(key string) (*[]Phraze, error) {
	var result []Phraze
	err := db.Model(&Phraze{}).Where("key = ?", key).Find(&result).Order("group ASC, order ASC").Error

	return &result, err
}

func getCensoredPharzesByKey(key string) (*[]Phraze, error) {
	var result []Phraze
	err := db.Model(&Phraze{}).Where("key = ? and is_uncensored = ?", key, true).Find(&result).Order("group ASC, order ASC").Error

	return &result, err
}

func GetPharzesByKey(key string, isUncensored bool) (*[]Phraze, error) {
	if isUncensored {
		return getAnyPharzesByKey(key)
	}

	return getCensoredPharzesByKey(key)
}
