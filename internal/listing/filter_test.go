// Tests for the visibility filter and the ".." entry.
package listing

import (
	"reflect"
	"strings"
	"testing"
)

func TestFilter(t *testing.T) {
	items := []Item{
		{Name: ".."},
		{Name: ".git", Attr: AttrDir | AttrHidden},
		{Name: "pagefile.sys", Attr: AttrHidden | AttrSystem},
		{Name: "boot.ini", Attr: AttrSystem},
		{Name: "run.sh", Attr: AttrExec},
		{Name: "setup.EXE"},
		{Name: "photo.jpg"},
		{Name: "pics", Attr: AttrDir},
		{Name: "node_modules", Attr: AttrDir},
	}
	jpg := func(it *Item) bool { return strings.HasSuffix(it.Name, ".jpg") }
	ignore := func(it *Item) bool { return it.Name == "node_modules" }
	tests := []struct {
		name string
		o    FilterOptions
		want []string
	}{
		{"default", FilterOptions{}, []string{"run.sh", "setup.EXE", "photo.jpg", "pics", "node_modules"}},
		{"up", FilterOptions{ShowUp: true}, []string{"..", "run.sh", "setup.EXE", "photo.jpg", "pics", "node_modules"}},
		{"up at root", FilterOptions{ShowUp: true, AtRoot: true}, []string{"run.sh", "setup.EXE", "photo.jpg", "pics", "node_modules"}},
		{"hidden", FilterOptions{ShowHidden: true}, []string{".git", "run.sh", "setup.EXE", "photo.jpg", "pics", "node_modules"}},
		{"hidden+system", FilterOptions{ShowHidden: true, ShowSystem: true}, []string{".git", "pagefile.sys", "boot.ini", "run.sh", "setup.EXE", "photo.jpg", "pics", "node_modules"}},
		{"programs", FilterOptions{Mode: ShowPrograms}, []string{"run.sh", "setup.EXE", "pics", "node_modules"}},
		{"custom", FilterOptions{Mode: ShowCustom, Match: jpg}, []string{"photo.jpg", "pics", "node_modules"}},
		{"custom dirs", FilterOptions{Mode: ShowCustom, Match: jpg, FilterDirs: true}, []string{"photo.jpg"}},
		{"ignore", FilterOptions{Ignore: ignore}, []string{"run.sh", "setup.EXE", "photo.jpg", "pics"}},
		{"ignore off", FilterOptions{Ignore: ignore, IgnoreOff: true}, []string{"run.sh", "setup.EXE", "photo.jpg", "pics", "node_modules"}},
	}
	for _, tt := range tests {
		if got := names(Filter(items, tt.o)); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s:\n got  %v\n want %v", tt.name, got, tt.want)
		}
	}
	if (&FilterOptions{}).Active() || !(&FilterOptions{Mode: ShowCustom}).Active() {
		t.Error("Active wrong")
	}
}
