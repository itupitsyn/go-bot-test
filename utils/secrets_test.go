package utils

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"
)

// Ровно та строка, из-за которой всё и делается: так библиотека печатает
// неудачный вызов Telegram, и так токен попадал в journalctl.
func TestMaskSecretsCutsTheTokenOutOfTheURL(t *testing.T) {
	line := `error do request for method sendMessage, ` +
		`Post "https://tg-api.example/bot5169125974:AAFnOPjdZXUoUshy468pqANPEJ/sendMessage": context canceled`

	got := MaskSecrets(line)

	if strings.Contains(got, "AAFnOPjdZXUoUshy468pqANPEJ") {
		t.Error("токен остался в строке")
	}
	if strings.Contains(got, "5169125974") {
		t.Error("числовая часть токена осталась в строке")
	}
	// Имя метода и причина — единственное, зачем эту строку читают. Если маска
	// съест и их, журнал станет бесполезен, и обёртку просто снимут.
	if !strings.Contains(got, "sendMessage") {
		t.Errorf("пропало имя метода: %s", got)
	}
	if !strings.Contains(got, "context canceled") {
		t.Errorf("пропала причина: %s", got)
	}
}

// Токен может приехать и не в URL — например, в отладочной строке библиотеки
// или в нашей собственной. Значение из окружения режем где угодно.
func TestMaskSecretsCutsValuesFromTheEnvironment(t *testing.T) {
	t.Setenv("AI_API_TOKEN", "очень-секретный-ключ")

	got := MaskSecrets("Authorization: Bearer очень-секретный-ключ")

	if strings.Contains(got, "очень-секретный-ключ") {
		t.Errorf("ключ остался: %s", got)
	}
	if !strings.Contains(got, "Authorization") {
		t.Errorf("вырезано лишнее: %s", got)
	}
}

// Самая опасная ошибка здесь — не пропустить секрет, а испортить журнал:
// ReplaceAll с пустой строкой вставляет маску между каждой парой символов.
// Незаданная переменная — обычное дело (DB_PASSWORD на стенде без пароля).
func TestMaskSecretsIgnoresEmptyAndShortValues(t *testing.T) {
	t.Setenv("TELEGRAM_APITOKEN", "")
	t.Setenv("AI_API_TOKEN", "1")
	t.Setenv("DB_PASSWORD", "")

	line := "Successfully connected to the database"
	if got := MaskSecrets(line); got != line {
		t.Errorf("строку изменили на пустых значениях: %s", got)
	}
}

// Паника приезжает в log одним куском со стеком, и маска обязана работать и
// посреди многострочного текста.
func TestMaskSecretsWorksAcrossLines(t *testing.T) {
	t.Setenv("TELEGRAM_APITOKEN", "5169125974:AAFnOPjdZXUoUshy468pqANPEJ")

	got := MaskSecrets("panic while handling an update\n" +
		"goroutine 1 [running]:\n" +
		"telebot/bot.send(5169125974:AAFnOPjdZXUoUshy468pqANPEJ)\n")

	if strings.Contains(got, "AAFnOPjdZXUoUshy468pqANPEJ") {
		t.Error("токен остался в стеке")
	}
	if !strings.Contains(got, "goroutine 1 [running]:") {
		t.Error("стек испорчен")
	}
}

// Проверка всей цепочки: log.Println(err) с токеном внутри обязан доехать до
// вывода уже без токена.
func TestHideSecretsInLogsMasksWhatWeActuallyPrint(t *testing.T) {
	var out bytes.Buffer

	previous := log.Writer()
	log.SetOutput(&out)
	defer log.SetOutput(previous)

	HideSecretsInLogs()
	log.Println(errors.New(`Post "https://tg-api.example/bot5169125974:AAFnOPjdZXUo/sendVideo": EOF`))

	printed := out.String()
	if strings.Contains(printed, "AAFnOPjdZXUo") {
		t.Errorf("токен доехал до вывода: %s", printed)
	}
	if !strings.Contains(printed, "sendVideo") {
		t.Errorf("строка потеряла смысл: %s", printed)
	}
}

// log считает запись неудачной, если вернуть меньше байт, чем дали, — а после
// вырезания их всегда меньше.
func TestMaskingWriterReportsTheLengthItWasGiven(t *testing.T) {
	t.Setenv("AI_API_TOKEN", "очень-секретный-ключ")

	var out bytes.Buffer
	w := maskingWriter{to: &out}

	line := []byte("ключ очень-секретный-ключ\n")
	n, err := w.Write(line)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if n != len(line) {
		t.Errorf("вернуло %d из %d — log решит, что запись неполная", n, len(line))
	}
	if out.Len() >= len(line) {
		t.Error("ничего не вырезано — тест ни о чём")
	}
}
