// This file implements the [=field] placeholder. In Total Commander the
// equals sign inserts a value from a content plugin, for example
// [=tc.size] or [=exif.DateTimeOriginal]. We ship a set of built-in fields
// computed from what every file system gives us (name parts, size, the
// modification date split into parts, the item type) and let the caller add
// any other value, such as EXIF or audio tags read elsewhere, through the
// Item.Fields map. Field names are matched without regard to case and an
// optional "tc." prefix is accepted so masks copied from Total Commander
// keep working. Unknown fields render as empty text and add a warning to the
// preview entry instead of stopping the whole preview.

package multirename

import (
	"sort"
	"strconv"
	"strings"
)

// fieldFunc computes a built-in field for the item being rendered.
type fieldFunc func(c *renderCtx) string

func sizeIn(unit int64) fieldFunc {
	return func(c *renderCtx) string { return strconv.FormatInt(c.item.Size/unit, 10) }
}

func timeFmt(layout string) fieldFunc {
	return func(c *renderCtx) string { return c.item.ModTime.Format(layout) }
}

// builtinFields maps lower-case field names to their implementation.
var builtinFields = map[string]fieldFunc{
	"name":         func(c *renderCtx) string { return c.name },
	"ext":          func(c *renderCtx) string { return c.ext },
	"fullname":     func(c *renderCtx) string { return c.item.Name },
	"path":         func(c *renderCtx) string { return c.item.Dir },
	"parent":       func(c *renderCtx) string { return c.parent },
	"grandparent":  func(c *renderCtx) string { return c.grand },
	"size":         sizeIn(1),
	"size.k":       sizeIn(1 << 10),
	"size.m":       sizeIn(1 << 20),
	"size.g":       sizeIn(1 << 30),
	"writedate":    func(c *renderCtx) string { return c.item.ModTime.Format(c.dateFormat()) },
	"writetime":    func(c *renderCtx) string { return c.item.ModTime.Format(c.timeFormat()) },
	"year":         timeFmt("2006"),
	"year2":        timeFmt("06"),
	"month":        timeFmt("01"),
	"monthname":    timeFmt("January"),
	"monthshort":   timeFmt("Jan"),
	"day":          timeFmt("02"),
	"hour":         timeFmt("15"),
	"minute":       timeFmt("04"),
	"second":       timeFmt("05"),
	"weekday":      timeFmt("Monday"),
	"weekdayshort": timeFmt("Mon"),
	"dayofyear":    func(c *renderCtx) string { return padInt(c.item.ModTime.YearDay(), 3) },
	"week": func(c *renderCtx) string {
		_, w := c.item.ModTime.ISOWeek()
		return padInt(w, 2)
	},
	"unixtime": func(c *renderCtx) string { return strconv.FormatInt(c.item.ModTime.Unix(), 10) },
	"type": func(c *renderCtx) string {
		if c.item.IsDir {
			return "folder"
		}
		return "file"
	},
	"index": func(c *renderCtx) string { return strconv.Itoa(c.position + 1) },
	"count": func(c *renderCtx) string { return strconv.Itoa(c.listLen) },
}

// FieldNames returns the names of the built-in fields, sorted, for the
// dialog's "Plugin" insert menu.
func FieldNames() []string {
	out := make([]string, 0, len(builtinFields))
	for k := range builtinFields {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// lookupField resolves a field for the item. Caller-supplied Item.Fields win
// over built-ins so the UI can override them (for example with an EXIF date
// in place of the file date).
func lookupField(name string, c *renderCtx) (string, bool) {
	if v, ok := c.item.Fields[name]; ok {
		return v, true
	}
	lname := strings.ToLower(name)
	for k, v := range c.item.Fields {
		if strings.ToLower(k) == lname {
			return v, true
		}
	}
	key := strings.TrimPrefix(lname, "tc.")
	if f, ok := builtinFields[key]; ok {
		return f(c), true
	}
	return "", false
}
