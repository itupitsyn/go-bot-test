package model

import (
	"telebot/database"

	"gorm.io/gorm/clause"
)

/**
 * Definitely not the best way to design this bit.
 * Using hardcoded IDs is ugly but kinda effective.
 */

type RoleType string

const (
	SuperAdminRole   RoleType = "SuperAdmin"
	PrizeCreatorRole RoleType = "PrizeCreator"
	PlayerRole       RoleType = "Player"
)

const (
	SuperAdminRoleID int64 = 1
	PrizeCreatorRoleID int64 = 2
	PlayerRoleID int64 = 3
)

type Role struct {
	ID            int64          `gorm:"primaryKey"`
	Name          string         `sql:"type:enum('SuperAdmin', 'PrizeCreator', 'Player');default:'Player'"`
	ChatUserRoles []ChatUserRole `gorm:"foreignKey:RoleID"`
}

func (role *Role) Save() (*Role, error) {
	err := database.Database.Save(&role).Error
	if err != nil {
		return &Role{}, err
	}
	return role, nil
}

func PopulateRoles() error {
	roles := []Role{
		{ID: SuperAdminRoleID, Name: string(SuperAdminRole)},
		{ID: PrizeCreatorRoleID, Name: string(PrizeCreatorRole)},
		{ID: PlayerRoleID, Name: string(PlayerRole)},
	}
	conflictClause := clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoNothing: true,
	}
	for _, role := range roles {
		result := database.Database.Clauses(conflictClause).Create(&role)
		if result.Error != nil {
			return result.Error
		}
	}
	return nil
}
