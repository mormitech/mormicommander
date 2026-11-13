package fileops

// Windows refuses some names that other systems accept: the characters < > : " | ? *, names ending in a dot or a
// space, and the old device names (CON, PRN, AUX, NUL, COM1–COM9, LPT1–LPT9, with or without an extension). Checking
// them before a rename gives the user a clear sentence instead of a cryptic system error.

import (
	"errors"
	"strings"
)

func checkNamePlatform(name string) error {
	if strings.ContainsAny(name, `<>:"|?*`) {
		return errors.New(`the name must not contain any of < > : " | ? *`)
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return errors.New("the name must not end with a dot or a space")
	}
	base, _, _ := strings.Cut(strings.ToUpper(name), ".")
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return errors.New("that name is reserved by Windows")
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
		return errors.New("that name is reserved by Windows")
	}
	return nil
}
