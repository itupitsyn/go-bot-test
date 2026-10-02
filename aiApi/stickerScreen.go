package aiApi

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"math"
)

// screenColour is the backdrop the pack asks the model for, and the word the
// model is told to call it.
//
// Why there is a choice at all. The service cuts the background out by colour,
// and colour cannot tell a green screen from a green hoodie: measured
// 02.10.2026 on a frame made on purpose (a bright green hoodie in front of a
// green screen), the hoodie sat 0.023-0.06 away from the screen in chromaticity
// — exactly where the screen's own spill in the hair sits. No threshold
// separates them. The way out is not to put the person and the backdrop in the
// same colour in the first place.
//
// RGB is what the model actually paints when asked for this screen, averaged
// over our own frames. It is used ONLY to choose; the service reads the real
// colour off the border of the frame it gets, so the two sides never have to
// agree on a number.
type screenColour struct {
	Word string
	RGB  [3]float64
}

// The list is short on purpose. Both colours have one clearly dominant channel
// — the service suppresses spill along that channel, and a colour mixed from
// two (magenta, orange) leaves it nothing to suppress. Both are also far from
// skin, which rules out the colours that would be safe for clothes and ruinous
// for a face.
//
// Green is first, so a photo with nothing green keeps the behaviour the pack
// had before this was added.
var screenColours = []screenColour{
	{Word: "green", RGB: [3]float64{60, 190, 70}},
	{Word: "blue", RGB: [3]float64{45, 75, 200}},
}

// clause is what goes into the prompt.
//
// The second half is not politeness but a patch over measured damage. On one
// seed in four the model dissolved an arm into the screen: the arm came out
// bright green, and cutting could do nothing about it because the defect was
// already in the generation. With this sentence, green on the person over the
// same four seeds went from 356, 16, 0, 0 pixels to 0, 0, 0, 11.
func (s screenColour) clause() string {
	return ", replace the background behind them with a flat solid chroma key " +
		s.Word + " screen, the " + s.Word + " must stay behind them and must " +
		"not touch the person"
}

// screenRiskDist is how close in chromaticity a pixel has to be to the screen
// for the service to be unable to tell them apart. It is the same 0.16 that
// cutout.py uses as _CHROMA_HI, and it is here for the same reason: past it,
// colour stops being evidence.
const screenRiskDist = 0.16

// screenRiskDark: below this sum of channels a pixel is too dark for its
// chromaticity to mean anything — black hair would otherwise read as whatever
// colour its noise leans towards.
const screenRiskDark = 60.0

// screenRiskSamples is how many pixels across the long side we look at. The
// answer is a share of the frame, and a share does not need every pixel; a
// photo from a phone would cost tens of milliseconds for no change in the
// verdict.
const screenRiskSamples = 192

// screenRiskMargin is how much better a later colour must score to displace
// green. Without it a photo with a trace of green in the corner would flip the
// whole pack to blue on noise.
const screenRiskMargin = 0.02

// screenFor picks the backdrop for this photo: the colour the person is least
// likely to be wearing.
//
// Looks at the WHOLE frame, not at the person. The bot has no mask, and the
// model redraws the scene rather than cutting the person out of it, so a colour
// anywhere in frame can end up on them. Being wrong here is cheap: both
// colours are valid screens, so an unnecessary switch to blue costs nothing.
//
// Any failure answers green, which is what the pack asked for before this
// existed.
func screenFor(photo []byte) screenColour {
	img, _, err := image.Decode(bytes.NewReader(photo))
	if err != nil {
		log.Println("[error] sticker pack: cannot read the photo for the screen colour", err)
		return screenColours[0]
	}

	best := 0
	risks := make([]float64, len(screenColours))
	for i, s := range screenColours {
		risks[i] = screenRisk(img, s)
		if i > 0 && risks[i] < risks[best]-screenRiskMargin {
			best = i
		}
	}

	log.Printf("sticker pack: screen = %s, risk %v\n", screenColours[best].Word, risks)

	return screenColours[best]
}

// screenRisk is the share of the photo that the service would not be able to
// tell from this screen.
func screenRisk(img image.Image, s screenColour) float64 {
	b := img.Bounds()
	side := b.Dx()
	if b.Dy() > side {
		side = b.Dy()
	}
	step := 1 + side/screenRiskSamples

	ref := chromaticity(s.RGB)
	near, total := 0, 0
	for y := b.Min.Y; y < b.Max.Y; y += step {
		for x := b.Min.X; x < b.Max.X; x += step {
			r, g, bb, _ := img.At(x, y).RGBA()
			px := [3]float64{float64(r >> 8), float64(g >> 8), float64(bb >> 8)}
			if px[0]+px[1]+px[2] < screenRiskDark {
				continue
			}

			total++
			if chromaGap(chromaticity(px), ref) < screenRiskDist {
				near++
			}
		}
	}

	if total == 0 {
		return 0
	}

	return float64(near) / float64(total)
}

// chromaticity is the colour with brightness divided out, the same way
// cutout.py does it: a shadow splits all three channels evenly, so a shaded
// screen and a lit one land in the same place.
func chromaticity(rgb [3]float64) [3]float64 {
	sum := rgb[0] + rgb[1] + rgb[2]
	if sum < 1 {
		sum = 1
	}

	return [3]float64{rgb[0] / sum, rgb[1] / sum, rgb[2] / sum}
}

func chromaGap(a, b [3]float64) float64 {
	gap := 0.0
	for i := 0; i < 3; i++ {
		if d := math.Abs(a[i] - b[i]); d > gap {
			gap = d
		}
	}

	return gap
}
