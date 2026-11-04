package aiApi

import (
	"fmt"
	"strings"
)

// languageNames maps a code from the Telegram profile to a language name for
// the prompt.
//
// The model can't be trusted to decode the tag: Qwen3.6 managed it, but 3.8
// answered "код IETF de" sometimes in Russian, sometimes in English, three
// misses out of three on the probe. Both understand a language name the same
// way, so this is settled here rather than left to the model.
//
// The value is a ready-made piece of the phrase "Отвечай на ...", which is why
// Hebrew and Hindi have no "языке": that is not how they are said.
var languageNames = map[string]string{
	"ru": "русском языке", "uk": "украинском языке", "be": "белорусском языке",
	"en": "английском языке", "de": "немецком языке", "fr": "французском языке",
	"es": "испанском языке", "it": "итальянском языке", "pt": "португальском языке",
	"pl": "польском языке", "cs": "чешском языке", "sr": "сербском языке",
	"bg": "болгарском языке", "ro": "румынском языке", "el": "греческом языке",
	"nl": "нидерландском языке", "sv": "шведском языке", "fi": "финском языке",
	"tr": "турецком языке", "ar": "арабском языке", "he": "иврите",
	"hy": "армянском языке", "ka": "грузинском языке", "az": "азербайджанском языке",
	"kk": "казахском языке", "uz": "узбекском языке", "ky": "киргизском языке",
	"tg": "таджикском языке", "hi": "хинди", "zh": "китайском языке",
	"ja": "японском языке", "ko": "корейском языке", "vi": "вьетнамском языке",
	"th": "тайском языке", "id": "индонезийском языке",
}

// languageName returns the language name, or an empty string if the code is
// unknown. Telegram also sends regional tags like "pt-BR", and only the base
// part matters to us.
func languageName(code string) string {
	return languageNames[baseLanguage(code)]
}

func baseLanguage(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	code, _, _ = strings.Cut(code, "-")

	return code
}

// explanationLabels names the two parts of an explanation of an image with text,
// for every language in languageNames: the labels are ours, not the model's, so
// they have to follow the asker's language here.
var explanationLabels = map[string][2]string{
	"ru": {"Текст", "Картинка"}, "uk": {"Текст", "Зображення"}, "be": {"Тэкст", "Малюнак"},
	"en": {"Text", "Image"}, "de": {"Text", "Bild"}, "fr": {"Texte", "Image"},
	"es": {"Texto", "Imagen"}, "it": {"Testo", "Immagine"}, "pt": {"Texto", "Imagem"},
	"pl": {"Tekst", "Obraz"}, "cs": {"Text", "Obrázek"}, "sr": {"Текст", "Слика"},
	"bg": {"Текст", "Изображение"}, "ro": {"Text", "Imagine"}, "el": {"Κείμενο", "Εικόνα"},
	"nl": {"Tekst", "Afbeelding"}, "sv": {"Text", "Bild"}, "fi": {"Teksti", "Kuva"},
	"tr": {"Metin", "Görsel"}, "ar": {"النص", "الصورة"}, "he": {"טקסט", "תמונה"},
	"hy": {"Տեքստ", "Նկար"}, "ka": {"ტექსტი", "სურათი"}, "az": {"Mətn", "Şəkil"},
	"kk": {"Мәтін", "Сурет"}, "uz": {"Matn", "Rasm"}, "ky": {"Текст", "Сүрөт"},
	"tg": {"Матн", "Тасвир"}, "hi": {"पाठ", "चित्र"}, "zh": {"文字", "图片"},
	"ja": {"テキスト", "画像"}, "ko": {"텍스트", "이미지"}, "vi": {"Văn bản", "Hình ảnh"},
	"th": {"ข้อความ", "รูปภาพ"}, "id": {"Teks", "Gambar"},
}

// ExplanationLabels returns the labels for the text and the image parts. With an
// empty or unknown languageCode they are Russian, as an image description is.
func ExplanationLabels(languageCode string) (string, string) {
	labels, ok := explanationLabels[baseLanguage(languageCode)]
	if !ok {
		labels = explanationLabels["ru"]
	}

	return labels[0], labels[1]
}

// summarySystemPrompt builds the system prompt for a retelling.
//
// languageCode is what Telegram puts in the profile's language_code. The field
// is optional and not always sent; for an empty or unknown value we ask for the
// retelling in the language of the text itself, which is more sensible than
// guessing.
func summarySystemPrompt(languageCode string) string {
	answerIn := "Отвечай на языке самого текста."
	if name := languageName(languageCode); name != "" {
		answerIn = fmt.Sprintf("Отвечай на %s.", name)
	}

	return "Перескажи текст пользователя коротко: два-четыре предложения, только суть. " +
		"Без вступлений вроде «в этом тексте говорится», без списков и без заголовка. " +
		"Если текст и так короткий, верни его почти как есть, не раздувая. " +
		answerIn + " Формат вывода — только пересказ."
}

func generateSummary(text, languageCode string) (string, error) {
	return askLLM(summarySystemPrompt(languageCode), text)
}
