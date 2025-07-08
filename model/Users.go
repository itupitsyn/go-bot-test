package model

import "time"

type User struct {
	ID              int64  `gorm:"primaryKey"`
	Name            string `gorm:"index:idx_name,size:255;not null" json:"name"`
	AlternativeName string `gorm:"size:255;not null;default:''"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       time.Time
	Raffle          []Raffle       `gorm:"foreignKey:WinnerID;"`
	Admins          []Admin        `gorm:"foreignKey:UserID;"`
	ChatUserRoles   []ChatUserRole `gorm:"foreignKey:UserID;"`
}

func (user *User) Save() (*User, error) {
	err := db.Save(&user).Error
	if err != nil {
		return &User{}, err
	}
	return user, nil
}

func (user *User) CanCreatePrize(chatID int64) bool {
	return db.Model(&ChatUserRole{}).Where(
		"user_id = ? AND chat_id = ? AND role_id IN (?, ?)", user.ID, chatID, SuperAdminRoleID, PrizeCreatorRoleID,
	).First(&ChatUserRole{}).Error == nil
}

func (user *User) IsSuperAdmin(chatID int64) bool {
	return db.Model(&ChatUserRole{}).Where(
		"user_id = ? AND chat_id = ? AND role_id = ?", user.ID, chatID, SuperAdminRoleID,
	).First(&ChatUserRole{}).Error == nil
}

func GetUserByName(userName string) (*User, error) {
	user := &User{}
	result := db.Model(&User{}).Where("name = ?", userName).First(user)

	return user, result.Error
}

func GetUsersByIds(ids []int64) []User {
	var users []User
	db.Find(&users, ids)

	return users
}
