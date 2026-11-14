// This file guards the Lister's side of the Wails iframe advisory (GHSA-2p5f-2gx8-mh8m, see docs/ARCHITECTURE.md,
// "Web view security"). The page draws every "document" kind file in an <embed> that loads a blob: address of the
// app's own origin, so a document type that can carry scripts (HTML, SVG, XML) must never be reported as a document:
// it could then run code that reaches the bound Go methods. SVG has to stay an "image" (drawn with <img>, which never
// runs scripts), and the only document type is PDF, matching the page's allowlist in viewer/mediael.js.

package lister

import (
	"bytes"
	"testing"
)

func TestMediaKindsAreSafeToEmbed(t *testing.T) {
	samples := [][]byte{
		[]byte("\x89PNG\r\n\x1a\n"), {0xFF, 0xD8, 0xFF}, []byte("GIF89a"), []byte("RIFF\x00\x00\x00\x00WEBPVP8X"),
		[]byte("II*\x00\xFF\xFF\xFF\xFF"), []byte("fLaC"), []byte("OggS"), []byte("ID3\x03\x00\x00\x7f\x7f\x7f\x7f"),
		{0x1A, 0x45, 0xDF, 0xA3, 0xFF}, []byte("\x00\x00\x00\x18ftypisom"), []byte("RIFF\x00\x00\x00\x00AVI LIST"),
		[]byte("RIFF\x00\x00\x00\x00WAVEfmt "), []byte("8BPS"), []byte("%PDF-1.7\n"),
		[]byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
	}
	sawPDF, sawSVG := false, false
	for _, b := range samples {
		m, err := ProbeMedia(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			t.Fatalf("%q: %v", b, err)
		}
		switch m.Kind {
		case "image", "audio", "video":
		case "document":
			if m.MIME != "application/pdf" {
				t.Fatalf("%q: document kind with MIME %q would be drawn in an <embed>", b, m.MIME)
			}
		default:
			t.Fatalf("%q: unexpected kind %q", b, m.Kind)
		}
		sawPDF = sawPDF || m.MIME == "application/pdf"
		if m.MIME == "image/svg+xml" {
			sawSVG = true
			if m.Kind != "image" {
				t.Fatalf("SVG must stay an image (drawn with <img>), got kind %q", m.Kind)
			}
		}
	}
	if !sawPDF || !sawSVG {
		t.Fatalf("samples did not cover PDF (%v) and SVG (%v)", sawPDF, sawSVG)
	}
}
