package model

type Chat struct {
	ID           int64          `gorm:"primaryKey"`
	Name         string         `gorm:"index:idx_name,size:255;not null;default:''" json:"name"`
	IsUncensored bool           `json:"is_uncensored"`
	Admin        []Admin        `gorm:"foreginKey:ChatID;"`
	Prize        []Prize        `gorm:"foreginKey:ChatID;"`
	Raffle       []Raffle       `gorm:"foreginKey:ChatID;"`
	ChatUserRole []ChatUserRole `gorm:"foreginKey:ChatID;"`
}

func (chat *Chat) Save() (*Chat, error) {
	err := db.Save(&chat).Error
	if err != nil {
		return &Chat{}, err
	}
	return chat, nil
}

func GetChatById(id int64) (*Chat, error) {
	result := &Chat{}

	err := db.Model(&Chat{}).Where("id = ?", id).Find(result).Error

	return result, err
}
