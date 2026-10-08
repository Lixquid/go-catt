// Magic-byte detection for files whose name gives no hint about their
// format. Extension-based detection happens first; when it fails (no
// extension, a generic one like .bin, or stdin input whose name is
// "<stdin>"), the leading bytes are sniffed so misnamed and extensionless
// files still get the rendering they deserve.
//
// Only formats with reliable magic bytes are detected here. Archives are
// sniffed separately (archiveFormatHead), and shell scripts are recognized
// from their #! shebang by chroma's content analysis, so neither is
// duplicated in this file. Markdown and CSV are plain text with no magic
// bytes and stay extension-only.
package main

import "bytes"

// imageMagicFormat identifies PNG, JPEG, and GIF images from their
// leading bytes, returning "png", "jpeg", or "gif". All three magics are
// long and specific enough to make false positives on other files
// practically impossible.
func imageMagicFormat(head []byte) string {
	switch {
	case len(head) >= 8 && bytes.Equal(head[:8], []byte("\x89PNG\r\n\x1a\n")):
		return "png"
	case len(head) >= 3 && bytes.Equal(head[:3], []byte("\xff\xd8\xff")):
		return "jpeg"
	case len(head) >= 6 && bytes.Equal(head[:6], []byte("GIF87a")),
		len(head) >= 6 && bytes.Equal(head[:6], []byte("GIF89a")):
		return "gif"
	}
	return ""
}

// isImageMagic reports whether data starts like a PNG, JPEG, or GIF image.
func isImageMagic(data []byte) bool {
	return imageMagicFormat(data) != ""
}
