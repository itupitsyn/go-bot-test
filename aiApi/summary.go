package aiApi

import "fmt"

// summarySystemPrompt собирает системный промпт для пересказа.
//
// languageCode — то, что Telegram кладёт в language_code профиля: тег IETF
// вроде "ru", "en" или "pt-BR". Поле необязательное и приходит не всегда,
// поэтому на пустое значение есть отдельная ветка: просить пересказ на языке
// самого текста разумнее, чем гадать.
func summarySystemPrompt(languageCode string) string {
	answerIn := "Отвечай на языке самого текста."
	if languageCode != "" {
		answerIn = fmt.Sprintf(
			"Отвечай на языке, которому соответствует код IETF «%s». "+
				"Если код незнаком, отвечай на языке самого текста.",
			languageCode,
		)
	}

	return "Перескажи текст пользователя коротко: два-четыре предложения, только суть. " +
		"Без вступлений вроде «в этом тексте говорится», без списков и без заголовка. " +
		"Если текст и так короткий, верни его почти как есть, не раздувая. " +
		answerIn + " Формат вывода — только пересказ."
}

func generateSummary(text, languageCode string) (string, error) {
	return askLLM(summarySystemPrompt(languageCode), text)
}
