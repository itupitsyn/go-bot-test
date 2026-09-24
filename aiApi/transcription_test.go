package aiApi

import (
	"encoding/json"
	"testing"
)

// parseTranscription builds a result from json so the tests speak the same
// language as the service.
func parseTranscription(t *testing.T, raw string) transcriptionResult {
	t.Helper()

	var result transcriptionResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("test data is not valid json: %v", err)
	}

	return result
}

func TestFormatTranscriptionSingleSpeakerHasNoLabels(t *testing.T) {
	// whisperx returns segment text with a leading space and cuts speech into
	// pieces more often than a person pauses.
	result := parseTranscription(t, `{"segments":[
		{"text":" Привет.","speaker":"SPEAKER_00"},
		{"text":" Как дела?","speaker":"SPEAKER_00"}
	],"language":"ru"}`)

	want := "Привет. Как дела?"
	if got := formatTranscription(result); got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestFormatTranscriptionLabelsSeveralSpeakers(t *testing.T) {
	result := parseTranscription(t, `{"segments":[
		{"text":" Привет.","speaker":"SPEAKER_00"},
		{"text":" Как дела?","speaker":"SPEAKER_00"},
		{"text":" Нормально.","speaker":"SPEAKER_01"},
		{"text":" А у тебя?","speaker":"SPEAKER_01"},
		{"text":" Тоже.","speaker":"SPEAKER_00"}
	],"language":"ru"}`)

	want := "Говорящий 1: Привет. Как дела?\n\n" +
		"Говорящий 2: Нормально. А у тебя?\n\n" +
		"Говорящий 1: Тоже."
	if got := formatTranscription(result); got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestFormatTranscriptionWithoutSpeakers(t *testing.T) {
	// Diarization may not have assigned speakers at all; then it is just text.
	result := parseTranscription(t, `{"segments":[
		{"text":" Раз."},
		{"text":" Два."}
	],"language":"ru"}`)

	want := "Раз. Два."
	if got := formatTranscription(result); got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestFormatTranscriptionSkipsEmptySegments(t *testing.T) {
	result := parseTranscription(t, `{"segments":[
		{"text":" Раз.","speaker":"SPEAKER_00"},
		{"text":"   ","speaker":"SPEAKER_00"},
		{"text":" Два.","speaker":"SPEAKER_00"}
	],"language":"ru"}`)

	want := "Раз. Два."
	if got := formatTranscription(result); got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestFormatTranscriptionEmpty(t *testing.T) {
	result := parseTranscription(t, `{"segments":[],"language":"ru"}`)

	if got := formatTranscription(result); got != "" {
		t.Errorf("want an empty string, got %q", got)
	}
}
