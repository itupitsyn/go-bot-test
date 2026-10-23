package bot

import (
	"strconv"
	"strings"
)

// How long a clip the animator will make. The length is asked for in words, at
// one of the ends of the command: "анимируй котика 10 секунд".
//
// videoSecondsMax is OUR ceiling and it has to match VIDEO_MAX_SECONDS on the
// service. The service clamps the request to its own range anyway, so a higher
// ceiling here would mean a person asking for fifteen seconds, getting ten and
// being charged for fifteen. Raising it is two edits, and only after a
// measurement: a frame gets dearer with length (2.3 s against 1.6 s on a
// five-second clip), so fifteen seconds is 17-19 minutes of one card and
// nobody has checked that it even fits.
//
// videoSecondsMin is the bottom of what the model was trained on. H3 has seen
// about no three-second clips, and the service would stretch such a request to
// five regardless; clamping here keeps the length we charge for equal to the
// length that goes on the card.
const (
	videoSecondsDefault = 5.0
	videoSecondsMin     = 5.0
	videoSecondsMax     = 10.0
)

// cutVideoSeconds reads the asked-for length out of the command text and
// returns the text without it. Zero means nothing was asked, and then the text
// comes back BYTE FOR BYTE: quotation marks and the spacing around them are
// what the speech in the video is assembled from, and re-joining the words
// would be a change nobody asked for.
//
// The length is only recognised at an end of the text, and only with a unit
// next to the number. In the middle it stays where it is: "как он 10 секунд
// стоит у двери" is a scene, not a length. The reading still is not free of
// doubt — "анимируй таймер, который показывает 10 секунд" loses its last two
// words — but at an edge, with a unit, that is as rare as the phrasing is, and
// a request for a longer clip has to be plain enough to charge for.
func cutVideoSeconds(text string) (float64, string) {
	fields := strings.Fields(text)

	// The tail comes first because that is where people write it, the same
	// place a style or a camera is added.
	if value, used, ok := secondsAtTail(fields); ok {
		return clampVideoSeconds(value), strings.Join(fields[:len(fields)-used], " ")
	}

	if value, used, ok := secondsAtHead(fields); ok {
		return clampVideoSeconds(value), strings.Join(fields[used:], " ")
	}

	return 0, text
}

// clampVideoSeconds brings the request down to what will actually go on the
// card. Refusing would be worse: the service clamps all the same, and the
// person asked for a clip, not for an error message. Whatever is charged later
// must be counted from this number, or thirty seconds asked and ten delivered
// would end up as thirty on the bill.
func clampVideoSeconds(seconds float64) float64 {
	return min(max(seconds, videoSecondsMin), videoSecondsMax)
}

// secondsAtHead reads a length written at the beginning of the text and says
// how many words it took.
func secondsAtHead(fields []string) (float64, int, bool) {
	if len(fields) == 0 {
		return 0, 0, false
	}

	first, ok := cleanToken(fields[0])
	if !ok {
		return 0, 0, false
	}

	if value, ok := gluedSeconds(first); ok {
		return value, 1, true
	}

	value, ok := secondsNumber(first)
	if !ok || len(fields) < 2 {
		return 0, 0, false
	}

	unit, ok := cleanToken(fields[1])
	if !ok || !isSecondsUnit(unit) {
		return 0, 0, false
	}

	return value, 2, true
}

// secondsAtTail does the same at the end of the text.
func secondsAtTail(fields []string) (float64, int, bool) {
	if len(fields) == 0 {
		return 0, 0, false
	}

	last, ok := cleanToken(fields[len(fields)-1])
	if !ok {
		return 0, 0, false
	}

	if value, ok := gluedSeconds(last); ok {
		return value, 1, true
	}

	if !isSecondsUnit(last) || len(fields) < 2 {
		return 0, 0, false
	}

	number, ok := cleanToken(fields[len(fields)-2])
	if !ok {
		return 0, 0, false
	}

	value, ok := secondsNumber(number)
	if !ok {
		return 0, 0, false
	}

	return value, 2, true
}

// cleanToken lowercases a word and strips the punctuation people put around
// it. A word carrying a quotation mark is refused outright: what stands in
// quotes is a spoken line, so «я подожду 10 секунд» is words to be said and
// not the length of the clip.
func cleanToken(token string) (string, bool) {
	if strings.ContainsAny(token, quotationMarks) {
		return "", false
	}

	return strings.TrimFunc(strings.ToLower(token), isCommandSeparator), true
}

// isSecondsUnit reports whether the word is a unit of time rather than a word
// of the scene. The Russian cases are taken by prefix ("секунд", "секунды",
// "секундочку"), the short forms are listed exactly: "с" as a prefix would
// swallow half the dictionary.
func isSecondsUnit(token string) bool {
	switch token {
	case "с", "сек", "s", "sec", "secs":
		return true
	}

	return strings.HasPrefix(token, "секунд") || strings.HasPrefix(token, "second")
}

// secondsNumber reads a plain count of seconds. Whole numbers only: nobody
// types "10.5 секунды", and parsing fractions would add a reading of its own
// (a comma against a dot) for a length that is not on the price list anyway.
func secondsNumber(token string) (float64, bool) {
	value, err := strconv.Atoi(token)
	if err != nil || value <= 0 {
		return 0, false
	}

	return float64(value), true
}

// gluedSeconds reads a number and a unit written as one word, "10с" or "10s".
// Written apart they are two words and the caller handles them.
func gluedSeconds(token string) (float64, bool) {
	cut := 0
	for cut < len(token) && token[cut] >= '0' && token[cut] <= '9' {
		cut++
	}
	if cut == 0 || cut == len(token) {
		return 0, false
	}

	value, ok := secondsNumber(token[:cut])
	if !ok || !isSecondsUnit(token[cut:]) {
		return 0, false
	}

	return value, true
}
