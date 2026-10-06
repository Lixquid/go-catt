// Custom chroma style derived from glamour's built-in "dark" markdown
// style. Glamour does not map its dark style onto one of chroma's
// standard styles (see ansi/codeblock.go, which registers the same
// entries at runtime under the name "charm"), so a standalone code
// file and a code block inside a markdown document would be styled
// differently. Registering the identical entries here makes catt's
// code file highlighting match glamour's markdown code blocks exactly.
package main

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
)

// cattDarkStyleName is the name under which the style below is
// registered with chroma.
const cattDarkStyleName = "catt-dark"

// Entries mirror glamour's dark.json "code_block.chroma" map. Empty
// entries in that map (e.g. name_constant) are omitted; chroma treats
// them as inherit-from-parent either way.
func init() {
	if _, ok := styles.Registry[cattDarkStyleName]; ok {
		return
	}
	styles.Register(chroma.MustNewStyle(cattDarkStyleName, chroma.StyleEntries{
		chroma.Text:                "#c4c4c4",
		chroma.Error:               "#f1f1f1 bg:#f05b5b",
		chroma.Comment:             "#676767",
		chroma.CommentPreproc:      "#ff875f",
		chroma.Keyword:             "#00aaff",
		chroma.KeywordReserved:     "#ff5fd2",
		chroma.KeywordNamespace:    "#ff5f87",
		chroma.KeywordType:         "#6e6ed8",
		chroma.Operator:            "#ef8080",
		chroma.Punctuation:         "#e8e8a8",
		chroma.Name:                "#c4c4c4",
		chroma.NameBuiltin:         "#ff8ec7",
		chroma.NameTag:             "#b083ea",
		chroma.NameAttribute:       "#7a7ae6",
		chroma.NameClass:           "#f1f1f1 underline bold",
		chroma.NameDecorator:       "#ffff87",
		chroma.NameFunction:        "#00d787",
		chroma.LiteralNumber:       "#6eefc0",
		chroma.LiteralString:       "#c69669",
		chroma.LiteralStringEscape: "#afffd7",
		chroma.GenericDeleted:      "#fd5b5b",
		chroma.GenericEmph:         "italic",
		chroma.GenericInserted:     "#00d787",
		chroma.GenericStrong:       "bold",
		chroma.GenericSubheading:   "#777777",
		chroma.Background:          "bg:#373737",
	}))
}
