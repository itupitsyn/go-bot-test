package model

type Admin struct {
	ChatID int64  `gorm:"primaryKey"`
	UserID *int64 `gorm:"primaryKey"`
}

func (admin *Admin) Save() (*Admin, error) {
	err := db.Save(&admin).Error

	if err != nil {
		return &Admin{}, err
	}

	return admin, nil
}

func (admin *Admin) IsAdmin() (bool, error) {
	err := db.First(admin).Error

	return err == nil, err
}
