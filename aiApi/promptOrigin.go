package aiApi

import (
	"encoding/json"
	"mime/multipart"
)

// promptOrigin is where a generated prompt came from: the text the person
// actually typed and the name of the template applied to it.
//
// It exists purely for statistics and changes nothing about generation. The
// service only ever sees the finished prompt -- cut down to the subject,
// translated into English and wrapped in a template -- and from that there is
// no way back to the original request. Collected without the pair, the prompts
// are a corpus of machine text: useless for building test cases when a new
// model shows up, which is the whole reason for keeping them.
//
// The style name earns its place separately: it is the only way to learn which
// templates people actually reach for and which are dead weight.
type promptOrigin struct {
	// Source is the raw message: before the trailing keyword was cut off,
	// before lowercasing and before translation.
	Source string
	// Style names the applied template. Empty means none was chosen.
	Style string
}

// statsJSON renders the fields as a tail for a hand-assembled JSON body, the
// same way Caller.userJSON does. Empty fields are left out entirely rather
// than sent as empty strings: the column should say "not reported", not
// "reported as nothing".
func (o promptOrigin) statsJSON() string {
	out := ""

	if o.Source != "" {
		if escaped, err := json.Marshal(o.Source); err == nil {
			out += `, "source_prompt": ` + string(escaped)
		}
	}

	if o.Style != "" {
		if escaped, err := json.Marshal(o.Style); err == nil {
			out += `, "style": ` + string(escaped)
		}
	}

	return out
}

// writeForm adds the same fields to a multipart request. Mirrors statsJSON for
// the endpoints that take a file.
func (o promptOrigin) writeForm(writer *multipart.Writer) error {
	if o.Source != "" {
		if err := writer.WriteField("source_prompt", o.Source); err != nil {
			return err
		}
	}

	if o.Style != "" {
		if err := writer.WriteField("style", o.Style); err != nil {
			return err
		}
	}

	return nil
}
