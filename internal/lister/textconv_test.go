// Tests for the HTML-to-text and RTF-to-text converters used by the
// Lister's "show as text" view of web pages and rich text documents.

package lister

import (
	"strings"
	"testing"
)

func TestHTMLToText(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"paragraphs", "<p>Hello   <b>bold</b>\n world</p><p>Second</p>", "Hello bold world\n\nSecond\n"},
		{"script and style dropped", "<head><title>T</title><style>p{}</style><script>alert('<p>')</script></head><body>Text</body>", "T\n\nText\n"},
		{"comments", "a<!-- <p>hidden</p> -->b", "ab\n"},
		{"entities", "<p>&lt;tag&gt; &amp; &eacute;&#337;&#x171; &nbsp;x</p>", "<tag> & éőű \u00a0x\n"},
		{"br", "one<br>two<br/>three", "one\ntwo\nthree\n"},
		{"lists", "<ul><li>a</li><li>b</li></ul><ol><li>x</li><li>y</li></ol>", "• a\n• b\n1. x\n2. y\n"},
		{"table", "<table><tr><td>a</td><td>b</td></tr><tr><th>c</th><td>d</td></tr></table>", "a\tb\nc\td\n"},
		{"pre kept", "<pre>  x  y\n z</pre>after", "  x  y\n z\n\nafter\n"},
		{"img alt", `<p>See <img src="x.png" alt="a cat"> here</p>`, "See [a cat] here\n"},
		{"quoted gt in attr", `<a title="x > y" href="#">link</a> text`, "link text\n"},
		{"inline no space", "<b>foo</b>bar <i>baz</i>", "foobar baz\n"},
		{"headings", "<h1>Title</h1>Body<h2>Sub</h2>", "Title\n\nBody\n\nSub\n"},
		{"uppercase tags", "<P>A</P><SCRIPT>x</SCRIPT><BR>B", "A\n\n\nB\n"},
		{"stray lt", "a < b and c", "a < b and c\n"},
		{"hr", "a<hr>b", "a\n────────────────────\nb\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HTMLToText(tt.in); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestHTMLCharset(t *testing.T) {
	tests := []struct {
		in   string
		want Encoding
		ok   bool
	}{
		{`<meta charset="windows-1250">`, Win1250, true},
		{`<META http-equiv="Content-Type" content="text/html; charset=ISO-8859-2">`, ISO8859_2, true},
		{`<meta charset=utf-8>`, UTF8, true},
		{`<p>none</p>`, "", false},
	}
	for _, tt := range tests {
		got, ok := HTMLCharset([]byte(tt.in))
		if got != tt.want || ok != tt.ok {
			t.Errorf("%q: %q %v", tt.in, got, ok)
		}
	}
}

func TestRTFToText(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"basic", `{\rtf1\ansi Hello {\b bold} world.\par Next line.}`, "Hello bold world.\nNext line.\n"},
		{"font table skipped", `{\rtf1{\fonttbl{\f0 Arial;}}{\colortbl;\red0\green0\blue0;}\f0 Text\par}`, "Text\n"},
		{"ansi codepage 1250", `{\rtf1\ansi\ansicpg1250 \'e1rv\'edzt\'fbr\'f5}`, "árvíztűrő\n"},
		{"default 1252", `{\rtf1\ansi caf\'e9}`, "café\n"},
		{"unicode with fallback", `{\rtf1\uc1 \u337?\u369?x}`, "őűx\n"},
		{"unicode uc2", `{\rtf1\uc2 \u8364\'80\'80 euro}`, "€ euro\n"},
		{"negative unicode", `{\rtf1 \u-3913?}`, "\uf0b7\n"},
		{"escapes", `{\rtf1 a\\b\{c\}d}`, "a\\b{c}d\n"},
		{"tab and symbols", `{\rtf1 a\tab b\emdash c\~d\ldblquote q\rdblquote}`, "a\tb—c\u00a0d“q”\n"},
		{"ignorable destination", `{\rtf1 {\*\generator Foo;}visible{\*\unknown hidden}}`, "visible\n"},
		{"pict skipped", `{\rtf1 before{\pict\pngblip 89504e47}after}`, "beforeafter\n"},
		{"source newlines ignored", "{\\rtf1 one\r\ntwo\\par\nthree}", "onetwo\nthree\n"},
		{"field result kept", `{\rtf1 {\field{\*\fldinst HYPERLINK "x"}{\fldrslt link text}}}`, "link text\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RTFToText([]byte(tt.in))
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
	if _, err := RTFToText([]byte("plain")); err != ErrNotRTF {
		t.Fatal("non-RTF accepted")
	}
	// Deeply unbalanced input must not panic.
	if _, err := RTFToText([]byte(`{\rtf1 }}}}{{{\'`)); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(HTMLToText(""), "\n") {
		t.Fatal("empty html")
	}
}
