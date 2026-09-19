package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// aiMaintenanceID — профилактика одна на весь бот, так что строка в таблице
// тоже одна. Заводит и правит её админка.
const aiMaintenanceID = 1

// AiMaintenance — профилактика AI-функций: пока она идёт, бот не ходит в
// сервисы генерации, а отвечает, что занят ремонтом и когда вернётся.
type AiMaintenance struct {
	ID        uint `gorm:"primaryKey"`
	IsEnabled bool `gorm:"not null;default:false"`
	// EndsAt — когда обещаем вернуться. Пусто — срок не назван.
	EndsAt    *time.Time
	UpdatedAt time.Time
}

// IsActive говорит, идёт ли профилактика в момент now. Названный срок
// заканчивает её сам: выключать руками, когда всё починили вовремя, не нужно.
func (m *AiMaintenance) IsActive(now time.Time) bool {
	if m == nil || !m.IsEnabled {
		return false
	}

	return m.EndsAt == nil || now.Before(*m.EndsAt)
}

// GetAiMaintenance возвращает настройку профилактики. Пока админка её ни разу
// не сохраняла, строки нет — это значит «профилактики нет», а не ошибка.
func GetAiMaintenance() (*AiMaintenance, error) {
	var maintenance AiMaintenance
	err := db.First(&maintenance, aiMaintenanceID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &maintenance, nil
}
