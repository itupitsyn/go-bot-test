package model

type Phraze struct {
	Key   string `gorm:"primaryKey;size:255;not null;"`
	Value string `gorm:"primaryKey;not null;"`
}

func GetPharzesByKey(key string) (*[]Phraze, error) {
	var result []Phraze
	err := db.Model(&Phraze{}).Where("key = ?", key).Find(&result).Error

	return &result, err
}
