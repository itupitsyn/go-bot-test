package bot

import (
	"fmt"
	"testing"
)

// Цена команды — это замеренное время карты, а не круглое число с потолка.
// Один балл это пятнадцать секунд; если таблицу поправят, пусть поправят и
// здесь, осознанно.
func TestAiPointsFollowsTheMeasuredCost(t *testing.T) {
	cases := map[string]int{
		aiKindEdit:    1,
		aiKindImage:   2,
		aiKindVideo:   10,
		aiKindSticker: 1,
	}

	for kind, want := range cases {
		if got := aiPoints(kind); got != want {
			t.Errorf("%s: получили %d, ждали %d", kind, got, want)
		}
	}
}

// Забытый в таблице вид работы должен стоить хоть что-то: ноль превратил бы
// новую команду в бесплатную дыру, и узнали бы мы об этом по счёту за свет.
func TestUnknownKindIsNotFree(t *testing.T) {
	if got := aiPoints("сочини симфонию"); got < 1 {
		t.Errorf("неизвестный вид стоит %d", got)
	}
}

// Удвоение длины ролика стоит ВТРОЕ дороже: 178 с карты на пяти секундах
// против 538 с на десяти, один и тот же холст. Линейная цена за секунду
// сделала бы длинный ролик самым выгодным — наоборот тому, что нужно.
func TestVideoLengthCostsMoreThanLinear(t *testing.T) {
	cases := map[float64]int{
		5:  10,
		10: 30,
		15: 57,
	}

	for seconds, want := range cases {
		if got := aiVideoPoints(seconds); got != want {
			t.Errorf("%.0f с: получили %d, ждали %d", seconds, got, want)
		}
	}

	// Та же мысль, но как свойство: секунда длинного ролика дороже секунды
	// короткого. Это выживет и смену показателя.
	short := float64(aiVideoPoints(5)) / 5
	long := float64(aiVideoPoints(10)) / 10
	if long <= short {
		t.Errorf("секунда десятисекундного (%.2f) не дороже секунды пятисекундного (%.2f)", long, short)
	}
}

// Шов между разбором просьбы и ценой: считают они от ОДНОГО числа. Длина
// зажата потолком там, где её читают, поэтому дороже ролика на потолке бот
// выписать не может, сколько бы человек ни попросил. Разъедься эти два места —
// и попросят минуту, получат десять секунд, а списано будет за минуту.
func TestNoClipCostsMoreThanTheCeiling(t *testing.T) {
	ceiling := aiVideoPoints(videoSecondsMax)

	for _, text := range []string{
		// Ровно потолок и заведомо больше него. Число берём из константы, а
		// не словом: когда потолок двигали с десяти на пятнадцать, строка
		// "котика 10 секунд" перестала быть потолком и уронила этот тест —
		// а проверяет он не десятку, а шов между разбором и ценой.
		fmt.Sprintf("котика %.0f секунд", videoSecondsMax),
		fmt.Sprintf("котика %.0f секунд", videoSecondsMax+1),
		"котика 100500 с",
	} {
		seconds, _ := cutVideoSeconds(text)
		if got := aiVideoPoints(seconds); got != ceiling {
			t.Errorf("%q: %d баллов, а потолок %d", text, got, ceiling)
		}
	}

	// Длины не просили — цена обычная, та же, что у короткого ролика.
	seconds, _ := cutVideoSeconds("котика")
	if got := aiVideoPoints(seconds); got != pointsVideo {
		t.Errorf("без длины %d баллов, ждали %d", got, pointsVideo)
	}
}

// Короче пяти секунд модель не умеет, и дешевле пятисекундного ничего нет:
// просьба про три секунды не должна выдавать цену ниже минимальной, иначе
// сервис сделает пять секунд, а возьмём мы за три.
func TestShorterThanDefaultCostsTheDefault(t *testing.T) {
	for _, seconds := range []float64{0, 1, 3, 5} {
		if got := aiVideoPoints(seconds); got != pointsVideo {
			t.Errorf("%.0f с: получили %d, ждали %d", seconds, got, pointsVideo)
		}
	}
}
