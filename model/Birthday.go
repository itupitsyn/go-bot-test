package model

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BirthdayReminder — запись «в этом чате напоминать про этого человека». Сама
// дата лежит у человека (users.birthday): бот не видит состава чата, поэтому
// список ведёт суперадмин, а дата у одного человека на все чаты одна.
//
// RemindedOn и CongratulatedOn — дата (ГГГГ-ММ-ДД), когда сообщение уже ушло.
// Дата, а не флаг: так на следующий год оно снова станет нужным само.
type BirthdayReminder struct {
	ChatID          int64  `gorm:"primaryKey;autoIncrement:false"`
	UserID          int64  `gorm:"primaryKey;autoIncrement:false"`
	RemindedOn      string `gorm:"size:10;not null;default:''"`
	CongratulatedOn string `gorm:"size:10;not null;default:''"`
	Chat            Chat   `gorm:"foreignKey:ChatID"`
	User            User   `gorm:"foreignKey:UserID"`
}

// BirthdayRow — напоминание вместе с тем, что нужно знать о человеке.
type BirthdayRow struct {
	ChatID          int64
	UserID          int64
	Name            string
	AlternativeName string
	Birthday        string
	RemindedOn      string
	CongratulatedOn string
}

// SetUserBirthday пишет дату (ММ-ДД) сырым запросом: через модель поле не
// записать, см. User.Birthday.
func SetUserBirthday(userID int64, day string) error {
	return db.Exec(`UPDATE users SET birthday = ? WHERE id = ?`, day, userID).Error
}

// AddBirthdayReminder не считает повторное добавление ошибкой: уже стоит — и хорошо.
func AddBirthdayReminder(chatID int64, userID int64) error {
	reminder := BirthdayReminder{ChatID: chatID, UserID: userID}

	return db.Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(&reminder).Error
}

// RemoveBirthdayReminder возвращает false, если такого напоминания и не было.
func RemoveBirthdayReminder(chatID int64, userID int64) (bool, error) {
	result := db.Where("chat_id = ? AND user_id = ?", chatID, userID).Delete(&BirthdayReminder{})

	return result.RowsAffected > 0, result.Error
}

const birthdayRowSelect = `
	SELECT r.chat_id, r.user_id, u.name, u.alternative_name, COALESCE(u.birthday, '') AS birthday,
		r.reminded_on, r.congratulated_on
	FROM birthday_reminders r
	JOIN users u ON u.id = r.user_id`

// GetChatBirthdays — список чата по дате. Человек без даты тоже в нём: дату могли
// стереть в админке, и суперадмин должен это видеть.
func GetChatBirthdays(chatID int64) ([]BirthdayRow, error) {
	var rows []BirthdayRow
	err := db.Raw(birthdayRowSelect+`
		WHERE r.chat_id = ?
		ORDER BY u.birthday ASC, u.name ASC`, chatID).Scan(&rows).Error

	return rows, err
}

// GetBirthdaysOn — напоминания на эти дни (ММ-ДД) в чатах, где они включены.
func GetBirthdaysOn(days []string) ([]BirthdayRow, error) {
	var rows []BirthdayRow
	err := db.Raw(birthdayRowSelect+`
		JOIN chats c ON c.id = r.chat_id
		WHERE c.birthdays_enabled AND u.birthday IN ?`, days).Scan(&rows).Error

	return rows, err
}

// MarkBirthdayReminded и MarkBirthdayCongratulated закрывают сообщение на эту дату.
func MarkBirthdayReminded(chatID int64, userID int64, date string) error {
	return markBirthday(chatID, userID, "reminded_on", date)
}

func MarkBirthdayCongratulated(chatID int64, userID int64, date string) error {
	return markBirthday(chatID, userID, "congratulated_on", date)
}

func markBirthday(chatID int64, userID int64, column string, date string) error {
	return db.Model(&BirthdayReminder{}).
		Where("chat_id = ? AND user_id = ?", chatID, userID).
		Update(column, date).Error
}

// IsNotFound нужен боту, чтобы отличить «такого человека нет» от сбоя базы.
func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
