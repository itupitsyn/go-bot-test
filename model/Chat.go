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

// SetUncensored flips only the censorship flag, so a stale chat can't
// overwrite whatever else the admin panel changed meanwhile.
func (chat *Chat) SetUncensored(isUncensored bool) error {
	err := db.Model(chat).Update("is_uncensored", isUncensored).Error
	if err == nil {
		chat.IsUncensored = isUncensored
	}
	return err
}

func GetChatById(id int64) (*Chat, error) {
	chat := &Chat{
		ID: id,
	}

	err := db.First(chat).Error

	return chat, err
}
