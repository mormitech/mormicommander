// The options of the Options dialog that are kept in the settings file but that the program does not act on yet
// (board issue #94). Most come from Total Commander's dialog, so a wincmd.ini copied over keeps its values, but the
// feature behind them is not there yet, or works without the option. The dialog marks these options as having no
// effect yet, and leaves them out with Misc.HideNoEffect. Their values are still loaded and saved as before.
//
// The list is checked by options_audit_test.go in the main package: an option the page or the Go code reads must not
// be here, and an option nobody reads must be. So when an option gets wired up, take it off this list; when a new
// option is added before its feature, put it on.

package config

// noEffect holds the "Page.Field" names of the options with no effect yet. Since 0.14.27 every option has its effect,
// so the list is empty; an option added before its feature goes here until the feature is there.
var noEffect = map[string]bool{}

// HasNoEffect says whether the program does not act on the option yet.
func HasNoEffect(name string) bool {
	f, ok := LookupSetting(name)
	return ok && f.NoEffect
}
