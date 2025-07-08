package model

import "gorm.io/gorm"

var db *gorm.DB

func Init(gormDB *gorm.DB) {
	db = gormDB
}
