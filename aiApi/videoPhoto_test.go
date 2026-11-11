package aiApi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// visionLLM stands in for the llm with eyes: it answers with the given text
// and remembers what it was asked.
func visionLLM(t *testing.T, answer string) *string {
	t.Helper()

	asked := new(string)
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			*asked = string(body)

			reply, err := json.Marshal(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]string{"content": answer}},
				},
			})
			if err != nil {
				t.Errorf("не собрался ответ подставной llm: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(reply)
		}))

	t.Cleanup(server.Close)
	t.Setenv("AI_LLM_URL", server.URL)

	return asked
}

func TestPhotoIsSentToTheModel(t *testing.T) {
	asked := visionLLM(t, "A woman at a kitchen table, waist up, facing the camera.")

	got := describeVideoPhoto([]byte("\xff\xd8\xff\x00фото"))

	if got != "A woman at a kitchen table, waist up, facing the camera." {
		t.Errorf("описание потерялось: %q", got)
	}
	if !strings.Contains(*asked, "data:") || !strings.Contains(*asked, "base64") {
		t.Errorf("картинка не уехала в модель: %.200s", *asked)
	}
}

// TestNoPhotoIsNotACall: t2v has no picture, and a call about nothing is a
// call for nothing.
func TestNoPhotoIsNotACall(t *testing.T) {
	asked := visionLLM(t, "should not be asked")

	if got := describeVideoPhoto(nil); got != "" {
		t.Errorf("без фотографии получили описание %q", got)
	}
	if *asked != "" {
		t.Errorf("без фотографии сходили в модель: %.80s", *asked)
	}
}

// TestBlindPlanningOnFailure: the plan worked blind until 11.11.2026 and made
// videos. Losing somebody's video because the describing call failed would be
// the worse trade.
func TestBlindPlanningOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
	t.Cleanup(server.Close)
	t.Setenv("AI_LLM_URL", server.URL)

	if got := describeVideoPhoto([]byte("фото")); got != "" {
		t.Errorf("при сбое вернулось %q, ждали пустую строку", got)
	}
}

// TestOverlongDescriptionIsDropped: the description is context for the plan,
// and a page of it would drown the request it is meant to serve.
func TestOverlongDescriptionIsDropped(t *testing.T) {
	visionLLM(t, strings.Repeat("слово ", 80))

	if got := describeVideoPhoto([]byte("фото")); got != "" {
		t.Errorf("слишком длинное описание не отброшено: %.60s", got)
	}
}

// TestPhotoReachesThePlan is the point of the whole thing: 101 video jobs of
// 164 in the journal are i2v, and until now the plan never saw the picture.
func TestPhotoReachesThePlan(t *testing.T) {
	asked := planLLM(t, `{"form":"speaks","scene":"A woman speaks","lines":["привет"],
		"camera":"a static shot","sound":"a quiet room"}`)

	_, _, err := buildVideoPrompt("анимируй «привет»", 5,
		"A woman at a kitchen table, facing the camera.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(*asked, "A woman at a kitchen table") {
		t.Errorf("описание фотографии не доехало до плана: %.400s", *asked)
	}
	if !strings.Contains(*asked, "первым кадром") {
		t.Errorf("план не знает, что фотография станет первым кадром: %.400s", *asked)
	}
}

// TestNoPhotoNoNote: a request without a picture must not be told about one.
func TestNoPhotoNoNote(t *testing.T) {
	asked := planLLM(t, `{"form":"silent","scene":"A cat","lines":[],
		"camera":"a static shot","sound":"a quiet room"}`)

	if _, _, err := buildVideoPrompt("кота", 5, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(*asked, "фотограф") {
		t.Errorf("плану померещилась фотография: %.300s", *asked)
	}
}
