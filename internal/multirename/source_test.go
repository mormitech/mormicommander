// Tests of the FieldSource hook: [=field] values the engine does not know itself, such as EXIF dates and audio tags,
// come from the caller; built-in fields and Item.Fields still win, and time values use the [d] and [t] layouts.

package multirename

import (
	"strings"
	"testing"
	"time"
)

func TestFieldSource(t *testing.T) {
	asked := 0
	src := func(it *Item, name string) (FieldValue, bool) {
		asked++
		switch name {
		case "audio.artist":
			return FieldValue{Text: "Band"}, true
		case "img.exifdate":
			return FieldValue{Time: time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC), IsTime: true}, true
		case "img.none":
			return FieldValue{IsTime: true}, true
		}
		return FieldValue{}, false
	}
	render := func(mask string, it Item) (string, []string) {
		s := DefaultSettings()
		s.NameMask = mask
		r, err := Compile(s)
		if err != nil {
			t.Fatal(err)
		}
		r.SetFieldSource(src)
		names, warns := r.NewNames([]Item{it})
		return names[0], warns[0]
	}
	it := newItem("song.mp3")
	if got, w := render("[=audio.artist] - [N]", it); got != "Band - song.mp3" || w != nil {
		t.Errorf("artist: %q %v", got, w)
	}
	if got, _ := render("[=tc.img.exifdate]", it); got != "2020-01-02 03.04.05.mp3" {
		t.Errorf("exif date: %q", got)
	}
	if got, w := render("x[=img.none]", it); got != "x.mp3" || w != nil {
		t.Errorf("empty time: %q %v", got, w)
	}
	if got, w := render("[=nope]", it); got != ".mp3" || len(w) != 1 || !strings.Contains(w[0], "nope") {
		t.Errorf("unknown field: %q %v", got, w)
	}
	asked = 0
	it.Fields = map[string]string{"audio.artist": "Mine"}
	if got, _ := render("[=audio.artist][=size]", it); got != "Mine5242880.mp3" || asked != 0 {
		t.Errorf("Item.Fields and built-ins win without asking the source: %q asked %d", got, asked)
	}
}
