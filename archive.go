// Archive rendering: list the contents of zip and tar archives
// (optionally gzip- or bzip2-compressed) as a tree of files, with
// 2 spaces of indentation per directory level.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

// archiveFormat identifies an archive by extension, falling back to
// magic bytes so that misnamed files (and stdin input) still work.
// gzip/bzip2 data is assumed to contain a tar archive.
func archiveFormat(name string, data []byte) string {
	ext := strings.ToLower(name)
	switch {
	case strings.HasSuffix(ext, ".zip"):
		return "zip"
	case strings.HasSuffix(ext, ".tar"),
		strings.HasSuffix(ext, ".tgz"),
		strings.HasSuffix(ext, ".tar.gz"),
		strings.HasSuffix(ext, ".tbz"),
		strings.HasSuffix(ext, ".tbz2"),
		strings.HasSuffix(ext, ".tar.bz2"):
		return "tar"
	}
	// Magic-byte sniffing.
	switch {
	case len(data) >= 4 && bytes.Equal(data[:4], []byte("PK\x03\x04")):
		return "zip"
	case len(data) >= 2 && bytes.Equal(data[:2], []byte("\x1f\x8b")):
		return "tar"
	case len(data) >= 3 && bytes.Equal(data[:3], []byte("BZh")):
		return "tar"
	case len(data) >= 262 && bytes.Equal(data[257:262], []byte("ustar")):
		return "tar"
	}
	return ""
}

// renderArchive lists the files in an archive as a tree, directories
// suffixed with "/", drawn with box-drawing characters and each level
// indented by 2 spaces.
func renderArchive(name string, data []byte, w io.Writer) error {
	format := archiveFormat(name, data)
	if format == "" {
		return fmt.Errorf("%s: unrecognized archive format", name)
	}

	var entries []string
	if format == "zip" {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return fmt.Errorf("%s: failed to read zip: %w", name, err)
		}
		for _, f := range zr.File {
			entries = append(entries, f.Name)
		}
	} else {
		var r io.Reader = bytes.NewReader(data)
		switch {
		case len(data) >= 2 && bytes.Equal(data[:2], []byte("\x1f\x8b")):
			gz, err := gzip.NewReader(r)
			if err != nil {
				return fmt.Errorf("%s: failed to read gzip: %w", name, err)
			}
			r = gz
		case len(data) >= 3 && bytes.Equal(data[:3], []byte("BZh")):
			r = bzip2.NewReader(r)
		}
		tr := tar.NewReader(r)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("%s: failed to read tar: %w", name, err)
			}
			entries = append(entries, hdr.Name)
		}
	}

	// Grey the box-drawing guides when output is styled, plain when
	// piped; CATT_COLOR=yes/no overrides, same as the other renderers.
	grey, reset := "", ""
	if useDarkStyle() {
		grey, reset = "\x1b[90m", "\x1b[0m"
	}
	printTree(buildTree(entries), w, "", grey, reset)
	return nil
}

// node is one entry in the archive tree.
type node struct {
	name     string
	isDir    bool
	children map[string]*node
}

// buildTree turns flat archive entry names into a nested tree. Names
// are cleaned of "./" prefixes and drive/absolute prefixes; directory
// entries are recorded as directories, and parent directories are
// created implicitly for deeper paths.
func buildTree(entries []string) *node {
	root := &node{name: "", isDir: true, children: map[string]*node{}}
	for _, e := range entries {
		e = strings.TrimPrefix(e, "./")
		e = strings.TrimPrefix(e, "/")
		wasDir := strings.HasSuffix(e, "/")
		e = strings.TrimSuffix(e, "/")
		if e == "" || e == "." {
			continue
		}
		// Drop any Windows drive prefix (e.g. "C:/") from zip entries.
		if len(e) > 3 && e[1] == ':' && (e[2] == '/' || e[2] == '\\') && isASCIIAlpha(e[0]) {
			e = e[3:]
		}
		cur := root
		parts := strings.Split(path.Clean(e), "/")
		for i, part := range parts {
			if part == "." || part == ".." {
				break
			}
			child, ok := cur.children[part]
			if !ok {
				child = &node{name: part, children: map[string]*node{}}
				cur.children[part] = child
			}
			if i < len(parts)-1 || (i == len(parts)-1 && wasDir) {
				child.isDir = true
			}
			cur = child
		}
	}
	return root
}

// isASCIIAlpha reports whether c is an ASCII letter.
func isASCIIAlpha(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// printTree writes the tree: directories first, then files, each
// alphabetically, with 3-character cells per level: branch markers
// (├─, └─ plus a space) before names, vertical guides (│) plus padding
// under directories that have following siblings, and blank padding
// otherwise.
func printTree(n *node, w io.Writer, prefix, grey, reset string) {
	names := make([]string, 0, len(n.children))
	for name := range n.children {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := n.children[names[i]], n.children[names[j]]
		if a.isDir != b.isDir {
			return a.isDir // directories before files
		}
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})

	for i, name := range names {
		child := n.children[name]
		branch := "├─"
		if i == len(names)-1 {
			branch = "└─"
		}
		if child.isDir {
			fmt.Fprintf(w, "%s%s%s%s %s/\n", grey, prefix, branch, reset, name)
			cont := "│  "
			if i == len(names)-1 {
				cont = "   "
			}
			printTree(child, w, prefix+cont, grey, reset)
		} else {
			fmt.Fprintf(w, "%s%s%s%s %s\n", grey, prefix, branch, reset, name)
		}
	}
}
