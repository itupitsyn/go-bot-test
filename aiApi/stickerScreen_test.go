package aiApi

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// photoOf paints a frame: the whole thing in fill, and a block of patch over
// part of it. share is how much of the width the patch takes.
func photoOf(t *testing.T, fill, patch color.RGBA, share float64) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 200, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			c := fill
			if float64(x) < 200*share {
				c = patch
			}
			img.Set(x, y, c)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

var (
	skin   = color.RGBA{R: 225, G: 180, B: 160, A: 255}
	hoodie = color.RGBA{R: 95, G: 200, B: 80, A: 255}
	denim  = color.RGBA{R: 55, G: 85, B: 170, A: 255}
	black  = color.RGBA{R: 8, G: 8, B: 10, A: 255}
)

// Человек в зелёном — экран обязан перестать быть зелёным. Это и есть всё, ради
// чего выбор цвета написан: зелёная толстовка на зелёном экране неотличима от
// фона по цвету, и вырезание на ней ломается.
func TestScreenAvoidsWhatThePersonWears(t *testing.T) {
	if got := screenFor(photoOf(t, skin, hoodie, 0.5)); got.Word != "blue" {
		t.Errorf("на зелёной толстовке хотим синий экран, получили %q", got.Word)
	}
}

// И наоборот: синяя одежда не должна уводить нас в синий экран.
func TestScreenAvoidsBlueClothesToo(t *testing.T) {
	if got := screenFor(photoOf(t, skin, denim, 0.5)); got.Word != "green" {
		t.Errorf("на синей одежде хотим зелёный экран, получили %q", got.Word)
	}
}

// Ничего опасного в кадре — остаёмся на зелёном, то есть на том, что набор
// просил до появления выбора.
func TestScreenStaysGreenWhenNothingClashes(t *testing.T) {
	if got := screenFor(photoOf(t, skin, skin, 0)); got.Word != "green" {
		t.Errorf("на обычной фотографии хотим зелёный экран, получили %q", got.Word)
	}
}

// Пятнышко зелёного в углу не должно переключать весь набор: см.
// screenRiskMargin.
func TestScreenDoesNotFlipOnATrace(t *testing.T) {
	if got := screenFor(photoOf(t, skin, hoodie, 0.01)); got.Word != "green" {
		t.Errorf("от следа зелёного экран менять не надо, получили %q", got.Word)
	}
}

// Тёмное не имеет цветности: у чёрных волос каналы шумят, и без отсева они
// читались бы как любой цвет подряд.
func TestScreenIgnoresTheDark(t *testing.T) {
	if got := screenFor(photoOf(t, skin, black, 0.6)); got.Word != "green" {
		t.Errorf("тёмное не должно решать за экран, получили %q", got.Word)
	}
}

// Картинку не прочитали — работаем как раньше, на зелёном. Набор не должен
// падать из-за выбора цвета.
func TestScreenFallsBackToGreen(t *testing.T) {
	if got := screenFor([]byte("не картинка")); got.Word != "green" {
		t.Errorf("на нечитаемой фотографии хотим зелёный, получили %q", got.Word)
	}
}

// Риск — это доля кадра, которую сервис не отличит от экрана. На своём же
// цвете она обязана быть почти единицей, на чужом — почти нулём: если это
// сломается, выбор станет случайным, а тесты выше всё равно пройдут.
func TestScreenRiskSeesItsOwnColour(t *testing.T) {
	img, _, err := image.Decode(bytes.NewReader(photoOf(t, hoodie, hoodie, 0)))
	if err != nil {
		t.Fatal(err)
	}

	if risk := screenRisk(img, screenColours[0]); risk < 0.9 {
		t.Errorf("зелёный кадр против зелёного экрана: риск %.2f, ждём почти 1", risk)
	}
	if risk := screenRisk(img, screenColours[1]); risk > 0.1 {
		t.Errorf("зелёный кадр против синего экрана: риск %.2f, ждём почти 0", risk)
	}
}
