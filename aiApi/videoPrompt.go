package aiApi

import "strings"

// Шаблоны движения для видео — то же, что стили у картинок, только полезная
// ось здесь не внешний вид, а движение и камера: у i2v картинка внешний вид
// уже задала, а у t2v его всё равно описывает сам запрос.
//
// Написаны предложениями, а не перечислением через запятую, потому что H3
// читает промпт энкодером Qwen3-VL 32B и сэмплится без guidance
// (BasicGuider, турбо-LoRA в 8 шагов) — «качественные» токены там не рычаг,
// а просто лишние слова.
//
// Звук упомянут в каждом шаблоне не для красоты: H3 генерит стерео в том же
// проходе, и дорожка в любом случае окажется в mp4. Не сказать про неё —
// значит отдать её на волю модели.
type videoStyle struct {
	// suffixes — чем пользователь включает шаблон. Ищутся в конце запроса,
	// как и у картинок: «анимируй кота вокруг».
	suffixes []string
	template string
}

// defaultVideoTemplate работает, когда ключевого слова нет.
const defaultVideoTemplate = "%s. Natural smooth motion, steady camera, consistent lighting. Ambient sound that matches the scene."

// defaultVideoSubject подставляется, если после срезки ключевого слова не
// осталось ничего — модели нужен хоть какой-то субъект.
const defaultVideoSubject = "the scene comes to life"

var videoStyles = []videoStyle{
	{
		[]string{" медленно", " slowly"},
		"%s. The action unfolds in slow motion, fluid and unhurried, the camera holds steady. Soft ambient sound.",
	},
	{
		[]string{" вокруг", " around"},
		"%s. The camera slowly orbits the subject in one smooth continuous arc. Ambient sound moves with the camera.",
	},
	{
		[]string{" ближе", " closer"},
		"%s. A slow cinematic push-in toward the subject, the framing gradually tightening. Ambient sound grows closer.",
	},
	{
		[]string{" дальше", " wider"},
		"%s. The camera slowly pulls back to reveal the surroundings. Ambient sound opens out.",
	},
	{
		[]string{" сверху", " above"},
		"%s. A sweeping aerial shot gliding over the scene, wind and open-air ambience.",
	},
	{
		[]string{" живо", " lively"},
		"%s. Quick confident movement, energetic action, the camera keeps up. Lively ambient sound.",
	},
}

// getVideoTemplate returns the template the trailing keyword asks for, or the
// default one when the request ends in no keyword at all.
func getVideoTemplate(msgText string) string {
	text := strings.ToLower(strings.TrimSpace(msgText))

	for _, style := range videoStyles {
		for _, suffix := range style.suffixes {
			if strings.HasSuffix(text, suffix) {
				return style.template
			}
		}
	}

	return defaultVideoTemplate
}

// getVideoPrompt cuts the trailing keyword off, leaving what the video is
// actually about. The template says the rest.
func getVideoPrompt(msgText string) string {
	text := strings.ToLower(strings.TrimSpace(msgText))

	for _, style := range videoStyles {
		for _, suffix := range style.suffixes {
			if cut, ok := strings.CutSuffix(text, suffix); ok {
				return strings.TrimSpace(cut)
			}
		}
	}

	return text
}

// buildVideoPrompt собирает финальный промпт: переводит то, что попросил
// пользователь, и оборачивает в шаблон выбранного движения.
func buildVideoPrompt(msgText string) (string, error) {
	template := getVideoTemplate(msgText)

	subject := getVideoPrompt(msgText)
	if subject == "" {
		subject = defaultVideoSubject
	}

	translated, err := translatePrompt(subject)
	if err != nil {
		return "", err
	}

	return applyPromptTemplate(template, translated), nil
}
