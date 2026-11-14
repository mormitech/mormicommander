package main

// The media fields of the Multi-Rename tool (F-506): [=img.width], [=img.exifdate], [=audio.artist] and friends, the
// values Total Commander users get from content plugins such as EXIF and ID3. They are the same facts the file list
// shows in its Pictures and Music columns (internal/listing reads them from the first bytes of the file), handed to
// the rename engine through its FieldSource hook. A file is read only when the mask names one of these fields, and
// only once per session: the preview runs on every keystroke, so the facts are cached by path, and a cached entry is
// read again when the file's size or time has changed. Folders, missing files and files without the facts give empty
// text, so a photo without EXIF simply gets an empty part in its new name instead of an error.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	lst "mormicommander/internal/listing"
	"mormicommander/internal/multirename"
)

// mrMediaField says how one field name is answered: from the image facts or the audio tags, and how to turn the
// cached value into text.
type mrMediaField struct {
	audio bool
	get   func(x map[string]any) multirename.FieldValue
}

func mrText(key string) func(map[string]any) multirename.FieldValue {
	return func(x map[string]any) multirename.FieldValue {
		s, _ := x[key].(string)
		return multirename.FieldValue{Text: s}
	}
}

func mrNum(key string, width int) func(map[string]any) multirename.FieldValue {
	return func(x map[string]any) multirename.FieldValue {
		n, ok := x[key].(int64)
		if !ok {
			return multirename.FieldValue{}
		}
		return multirename.FieldValue{Text: fmt.Sprintf("%0*d", width, n)}
	}
}

// mrExif gives the EXIF date as a time (written with the dialog's [d] and [t] layouts) or, with a layout, one part.
func mrExif(layout string) func(map[string]any) multirename.FieldValue {
	return func(x map[string]any) multirename.FieldValue {
		t, ok := x[lst.ExtraExifDate].(time.Time)
		if !ok || t.IsZero() {
			return multirename.FieldValue{}
		}
		if layout == "" {
			return multirename.FieldValue{Time: t, IsTime: true}
		}
		return multirename.FieldValue{Text: t.Format(layout)}
	}
}

// mrMediaFields lists the field names in lower case. The names shown in the dialog's field menu come first (see
// mrMediaFieldNames); "exif.*" and "id3.*" are accepted too, so masks written for TC's plugins keep working.
var mrMediaFields = map[string]mrMediaField{
	"img.width":      {get: mrNum(lst.ExtraImgWidth, 0)},
	"img.height":     {get: mrNum(lst.ExtraImgHeight, 0)},
	"img.size":       {get: mrImgSize},
	"img.exifdate":   {get: mrExif("")},
	"img.exifyear":   {get: mrExif("2006")},
	"img.exifmonth":  {get: mrExif("01")},
	"img.exifday":    {get: mrExif("02")},
	"img.exifhour":   {get: mrExif("15")},
	"img.exifminute": {get: mrExif("04")},
	"img.exifsecond": {get: mrExif("05")},
	"audio.title":    {audio: true, get: mrText(lst.ExtraTitle)},
	"audio.artist":   {audio: true, get: mrText(lst.ExtraArtist)},
	"audio.album":    {audio: true, get: mrText(lst.ExtraAlbum)},
	"audio.year":     {audio: true, get: mrText(lst.ExtraYear)},
	"audio.track":    {audio: true, get: mrNum(lst.ExtraTrack, 2)},
	"audio.genre":    {audio: true, get: mrText(lst.ExtraGenre)},

	"exif.datetimeoriginal": {get: mrExif("")},
	"exif.datetime":         {get: mrExif("")},
	"exif.imagewidth":       {get: mrNum(lst.ExtraImgWidth, 0)},
	"exif.imageheight":      {get: mrNum(lst.ExtraImgHeight, 0)},
	"id3.title":             {audio: true, get: mrText(lst.ExtraTitle)},
	"id3.artist":            {audio: true, get: mrText(lst.ExtraArtist)},
	"id3.album":             {audio: true, get: mrText(lst.ExtraAlbum)},
	"id3.year":              {audio: true, get: mrText(lst.ExtraYear)},
	"id3.track":             {audio: true, get: mrNum(lst.ExtraTrack, 2)},
	"id3.genre":             {audio: true, get: mrText(lst.ExtraGenre)},
}

// mrMediaFieldNames are the media fields offered in the dialog's field menu, in menu order.
var mrMediaFieldNames = []string{
	"img.exifdate", "img.exifyear", "img.exifmonth", "img.exifday", "img.exifhour", "img.exifminute", "img.exifsecond",
	"img.width", "img.height", "img.size",
	"audio.artist", "audio.title", "audio.album", "audio.track", "audio.year", "audio.genre",
}

func mrImgSize(x map[string]any) multirename.FieldValue {
	w, ok1 := x[lst.ExtraImgWidth].(int64)
	h, ok2 := x[lst.ExtraImgHeight].(int64)
	if !ok1 || !ok2 {
		return multirename.FieldValue{}
	}
	return multirename.FieldValue{Text: strconv.FormatInt(w, 10) + "x" + strconv.FormatInt(h, 10)}
}

// mrMediaEntry is the cached facts of one file, with the size and time they were read at.
type mrMediaEntry struct {
	item    lst.Item
	size    int64
	modTime time.Time
}

// mrMediaCache answers media fields for the rename engine. It is used under MultiRename.mu, so it needs no lock.
type mrMediaCache struct {
	entries map[string]*mrMediaEntry
}

// source is the FieldSource for the engine.
func (c *mrMediaCache) source(it *multirename.Item, name string) (multirename.FieldValue, bool) {
	f, ok := mrMediaFields[name]
	if !ok {
		return multirename.FieldValue{}, false
	}
	if it.IsDir {
		return multirename.FieldValue{}, true
	}
	e := c.entry(it.Path())
	if e == nil {
		return multirename.FieldValue{}, true
	}
	if f.audio {
		lst.LoadAudioFacts(&e.item)
	} else {
		lst.LoadImageFacts(&e.item)
	}
	return f.get(e.item.Extra), true
}

// entry gives the cache entry of a regular file, read afresh when the file changed; nil for anything else.
func (c *mrMediaCache) entry(path string) *mrMediaEntry {
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return nil
	}
	if c.entries == nil {
		c.entries = map[string]*mrMediaEntry{}
	}
	if e := c.entries[path]; e != nil && e.size == fi.Size() && e.modTime.Equal(fi.ModTime()) {
		return e
	}
	e := &mrMediaEntry{size: fi.Size(), modTime: fi.ModTime()}
	e.item = lst.Item{Name: filepath.Base(path), Dir: filepath.Dir(path), Size: fi.Size(), ModTime: fi.ModTime(), Mode: fi.Mode()}
	c.entries[path] = e
	return e
}
