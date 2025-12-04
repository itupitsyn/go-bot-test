package model

type Phraze struct {
	Key           string `gorm:"primaryKey;size:255;not null;"`
	Value         string `gorm:"primaryKey;not null;"`
	IsUncensored  bool   `json:"is_uncensored"`
	IsWithSpoiler bool   `gorm:"default:false" json:"is_with_spoiler"`
	Group         *int
	Order         *int
}

func GetPharzesByKey(key string, isUncensord bool) (*[]Phraze, error) {
	var result []Phraze
	err := db.Model(&Phraze{}).Where("key = ? and is_uncensored = ?", key, isUncensord).Find(&result).Order("group ASC, order ASC").Error

	return &result, err
}
