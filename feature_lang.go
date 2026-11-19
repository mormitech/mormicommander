// Translations of the interface (Options > Language, issue #113). A language file maps English texts to translations;
// the frontend's t() (frontend/src/core/i18n.js) looks every text up and keeps the English one when the file has none,
// so without a file, or for a word the file lacks, the interface is the English it always was.
//
// Two formats are read:
//   - .lng (or any other name): lines "English text=Translated text"; ";" and "#" start a comment; an "=" inside the
//     English text is written "\=" and a line break "\n"; lines in a "[hu]" section count only when the interface language is hu, lines
//     outside any section always.
//   - .json: {"English text": "Translated text", ...}; an object value ("hu": {...}) is a section like "[hu]".
//
// An empty translation counts as none, so the template (frontend: npm run lang:template) works as a language file
// that changes nothing until it is filled in. The file is looked for in the settings folder first; a plain name that
// isn't there may name one of the files that come with mormiCommander (lang/*.lng, built in). With no file set, an
// interface language that has a built-in file ("hu") uses it; "en", the default, has none.

package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	"mormicommander/internal/config"
)

//go:embed lang/*.lng
var bundledLangs embed.FS

// bundledLanguage is the built-in language file of that name ("hu.lng", or just "hu"), or nil.
func bundledLanguage(name string) []byte {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || strings.ContainsAny(name, `/\`) || name == "template" || name == "template.lng" {
		return nil
	}
	for _, n := range []string{name, name + ".lng"} {
		if b, err := bundledLangs.ReadFile(path.Join("lang", n)); err == nil {
			return b
		}
	}
	return nil
}

// Language is the translation table of Options > Language (see the top of the file). Empty when no file is set and
// the interface language has no built-in file; an error when the file set can't be read.
func (c *Configuration) Language() (map[string]string, error) {
	cfg, err := c.get()
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(cfg.Settings.Language.LanguageFile)
	lang := strings.ToLower(strings.TrimSpace(cfg.Settings.Language.UILanguage))
	full := ""
	if name != "" {
		full = cfg.Path(name)
	}
	c.mu.Unlock()
	data, src, err := languageSource(name, full, lang)
	if err != nil || data == nil {
		return map[string]string{}, err
	}
	return parseLanguage(data, src, lang)
}

// languageSource finds the language file: the file set (full is its path in the settings folder), else a built-in one
// of that name, else (no file set) the built-in file of the interface language. nil data means English.
func languageSource(name, full, lang string) ([]byte, string, error) {
	if name == "" {
		if b := bundledLanguage(lang); b != nil {
			return b, lang + ".lng", nil
		}
		return nil, "", nil
	}
	data, err := os.ReadFile(full)
	if err == nil {
		return data, full, nil
	}
	if b := bundledLanguage(name); b != nil {
		return b, name, nil
	}
	return nil, "", fmt.Errorf("The language file %s can't be read: %v", full, err)
}

// parseLanguage reads a language file of either format into English → translation.
func parseLanguage(data []byte, name, lang string) (map[string]string, error) {
	text, _ := config.Decode(data, config.EncCP1252)
	out := map[string]string{}
	put := func(k, v string) {
		if k, v = strings.TrimSpace(k), strings.TrimSpace(v); k != "" && v != "" {
			out[k] = v
		}
	}
	trimmed := strings.TrimSpace(text)
	if strings.HasSuffix(strings.ToLower(name), ".json") || strings.HasPrefix(trimmed, "{") {
		var m map[string]any
		if err := json.Unmarshal([]byte(trimmed), &m); err != nil {
			return nil, fmt.Errorf("The language file %s is not valid JSON: %v", name, err)
		}
		var sec map[string]any
		for k, v := range m {
			switch x := v.(type) {
			case string:
				put(k, x)
			case map[string]any:
				if strings.EqualFold(k, lang) {
					sec = x
				}
			}
		}
		for k, v := range sec { // (the language's own section wins over the lines outside any section)
			if s, ok := v.(string); ok {
				put(k, s)
			}
		}
		return out, nil
	}
	section := ""
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == ';' || line[0] == '#' {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		if section != "" && section != lang {
			continue
		}
		if k, v, ok := cutUnescaped(line); ok {
			nl := strings.NewReplacer(`\n`, "\n")
			put(nl.Replace(k), nl.Replace(v))
		}
	}
	return out, nil
}

// cutUnescaped splits "key=value" at the first "=" that is not written "\=", and turns "\=" in the key back into "=".
func cutUnescaped(line string) (string, string, bool) {
	for i := 0; i < len(line); i++ {
		if line[i] == '\\' && i+1 < len(line) && line[i+1] == '=' {
			i++
			continue
		}
		if line[i] == '=' {
			return strings.ReplaceAll(line[:i], `\=`, "="), line[i+1:], true
		}
	}
	return "", "", false
}
