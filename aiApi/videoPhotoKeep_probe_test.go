package aiApi

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

// A probe, not a test: it runs the plan on i2v requests from the journal
// (13–16.11.2026) where the scene came back as a description of the photo with
// the request thrown out — "в кадр заходит толстый пьяный мужик…" over a photo
// of a desk became "a sign on a desk… No people are present". The photo
// descriptions are rebuilt from the opening of what went to the generator, in
// the describer's own style. Run:
//
//	AI_PHOTOKEEP_PROBE=3 go test ./aiApi/ -run TestVideoPhotoKeepProbe -v -timeout 30m
//
// The number is how many times each case is planned: the defect floats.
func TestVideoPhotoKeepProbe(t *testing.T) {
	runs, _ := strconv.Atoi(os.Getenv("AI_PHOTOKEEP_PROBE"))
	if runs <= 0 {
		t.Skip("AI_PHOTOKEEP_PROBE не задан (в нём число прогонов)")
	}
	if os.Getenv("AI_LLM_URL") == "" {
		_ = godotenv.Load("../.env.local")
	}
	if os.Getenv("AI_LLM_URL") == "" {
		t.Skip("AI_LLM_URL не задан — пробнику некуда ходить")
	}

	dog := "A close-up profile of a fluffy tan and white dog resting on a patterned blanket indoors, eyes closed, mouth wide open in a yawn."
	cases := []struct {
		request, photo string
		want           string // what the scene has to mention, as a regexp
		notNarrator    bool
	}{
		{"в кадр входит Путин и обявляет об окончании СВО, выходит солнце и все радуются и обнимаются.",
			"A wide aerial view of a sprawling city at sunset: brick industrial buildings and smokestacks, a train station, distant skyscrapers. No people are visible.",
			`(?i)putin`, false},
		{"в кадр заходит толстый пьяный мужик, садится за стол и закуривает сигарету. Затем мужик плачет.",
			"A close-up of a handwritten wooden sign on a desk in a dimly lit room, a lit cigarette beside it with smoke rising. No people are present.",
			`(?i)(man|guy).*(cr(y|ies)|tears|weep|sob)`, false},
		{"в кадр входят китайские модели и мужик начинает с ними экспериментировать.",
			"A four-panel meme of a gray-haired man with glasses and a woman with a beaded headpiece, close-ups against a green grassy background.",
			`(?i)chinese`, false},
		{"в кадр заходит света",
			"A close-up of a microphone on a tangle of black cables under a fluorescent light, the red word 'Херня' behind it. No people are present.",
			`(?i)sveta|svetlana`, false},
		{"собака читает Коран вслух", dog, `(?i)qur.?an|koran`, false},
		{"собака призывает к вечернему намазу", dog, `(?i)prayer|namaz|azan|adhan`, false},
		{"собаку целует в засос Александр Лукашенко, после чего дарит ей картофель, поворачивает ее и шлепает по попе.",
			dog, `(?i)lukashenko.*potato`, true},
		{"в кадр залетает птичка и сренькает на человека",
			"A man in a dark suit reclines on grass holding a paintbrush and gazes upward, a painted riverside landscape with figures and trees behind him.",
			`(?i)poop|droppings|defecat|excrement|shits|relieves`, false},
		{"в щель обведённую красным ставится складной тазик",
			"A small bathroom: a white bathtub with bronze fixtures, a toilet, black-and-white patterned floor tiles, a red outline drawn around a gap near the tub.",
			`(?i)(basin|tub|bowl).*(gap|slot|space)|(gap|slot|space).*(basin|bowl)`, true},
		{"в кадр заходит мопс",
			"A wide shot of a room with patterned wallpaper and sheer white curtains, a black metal bed frame on a light wooden floor. No people are present.",
			`(?i)pug`, false},
	}

	total, good := 0, 0
	for _, c := range cases {
		re := regexp.MustCompile(c.want)
		hits := 0
		for i := 0; i < runs; i++ {
			plan, err := planVideo(c.request, 5, c.photo)
			if err != nil {
				t.Errorf("%q: %v", c.request, err)
				continue
			}
			text := plan.Scene + " " + strings.Join(plan.Lines, " ")
			ok := re.MatchString(plan.Scene) && !(c.notNarrator && plan.Form == formNarrator)
			if ok {
				hits++
			}
			mark := "ПЛОХО"
			if ok {
				mark = "ок"
			}
			t.Logf("[%s] %-8s %.70s\n      %s", mark, plan.Form, c.request, text)
		}
		total += runs
		good += hits
	}
	t.Logf("ИТОГО: %d из %d", good, total)
}
