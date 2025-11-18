package model

type ChatUserRole struct {
	UserID        int64 `gorm:"primaryKey"`
	ChatID        int64 `gorm:"primaryKey"`
	RoleID        int64
	IsSetManually bool `gorm:"default:false"`
}

func (chatUserRole *ChatUserRole) Save() (*ChatUserRole, error) {
	err := db.Save(&chatUserRole).Error
	if err != nil {
		return &ChatUserRole{}, err
	}
	return chatUserRole, nil
}

func IsSuperAdmin(chatId int64, userId int64) (bool, error) {
	superAdminChatUserRole := ChatUserRole{}
	result := db.Where(
		"chat_id = ? AND user_id = ? AND role_id = ?", chatId, userId, SuperAdminRoleID,
	).First(&superAdminChatUserRole)

	return result.Error == nil, result.Error
}

func GetFirstChatUserRole(chatId int64, userId int64) (*ChatUserRole, error) {
	chatUserRole := &ChatUserRole{
		ChatID: chatId,
		UserID: userId,
	}
	chatUserRoleResult := db.First(&chatUserRole)

	return chatUserRole, chatUserRoleResult.Error
}

func GetChatAdmins(chatId int64) ([]ChatUserRole, error) {
	var chatUserRoles []ChatUserRole
	chatUserRoleResult := db.Where(
		"chat_id = ? AND role_id IN (?, ?)", chatId, SuperAdminRoleID, PrizeCreatorRoleID,
	).Order("role_id asc").Find(&chatUserRoles)

	return chatUserRoles, chatUserRoleResult.Error
}

func (chatUserRole *ChatUserRole) DeleteChatUserRole() error {
	result := db.Delete(&chatUserRole)
	return result.Error
}
