// Archive rendering: list the contents of zip and tar archives
// (optionally gzip- or bzip2-compressed) as a tree of files, with
// 2 spaces of indentation per directory level. Archives are never
// loaded into memory in full: tar-family archives are read as a
// stream, and zip archives are read via a ReaderAt so only the
// central directory is touched.
package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
)

// archiveHeadSize is the number of leading bytes needed to sniff an
// archive's magic (the ustar signature sits at offset 257).
const archiveHeadSize = 262

// archiveSource describes an archive to list: name for error
// messages, format ("zip" or "tar"), and the input. Tar-family
// archives are read sequentially from r; zip archives use ra to read
// the central directory without touching file contents.
type archiveSource struct {
	name   string
	format string
	r      io.Reader
	ra     io.ReaderAt
	size   int64
}

// archiveFormatName identifies an archive by extension.
func archiveFormatName(name string) string {
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
	return ""
}

// archiveFormatHead identifies an archive from its leading bytes so
// that misnamed files (and stdin input) still work. gzip/bzip2 data
// is assumed to contain a tar archive.
func archiveFormatHead(head []byte) string {
	// Magic-byte sniffing.
	switch {
	case len(head) >= 4 && bytes.Equal(head[:4], []byte("PK\x03\x04")):
		return "zip"
	case len(head) >= 2 && bytes.Equal(head[:2], []byte("\x1f\x8b")):
		return "tar"
	case len(head) >= 3 && bytes.Equal(head[:3], []byte("BZh")):
		return "tar"
	case len(head) >= 262 && bytes.Equal(head[257:262], []byte("ustar")):
		return "tar"
	}
	return ""
}

// renderArchiveFile lists the archive in f, which must be positioned
// at the start of the archive. Files are seekable, so zip archives
// are read in place; tar-family archives stream.
func renderArchiveFile(name string, f *os.File, format string, w io.Writer) error {
	src := archiveSource{name: name, format: format, r: f}
	if format == "zip" {
		info, err := f.Stat()
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		src.ra, src.size, src.r = f, info.Size(), nil
	}
	return renderArchive(src, w)
}

// renderArchiveStream lists an archive arriving over a non-seekable
// reader such as stdin. zip needs a seekable ReaderAt, so the stream
// is spooled to a temporary file first; tar-family archives stream
// directly. r must already include any sniffed prefix bytes.
func renderArchiveStream(name, format string, r io.Reader, w io.Writer) error {
	if format != "zip" {
		return renderArchive(archiveSource{name: name, format: format, r: r}, w)
	}
	tf, err := os.CreateTemp("", "catt-archive-")
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	defer os.Remove(tf.Name())
	defer tf.Close()
	size, err := io.Copy(tf, r)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if _, err := tf.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return renderArchive(archiveSource{name: name, format: format, ra: tf, size: size}, w)
}

// archiveEntry is one flat entry read from an archive: its path name
// (as stored, possibly with a trailing "/" for directories) and the
// uncompressed size of its contents.
type archiveEntry struct {
	name string
	size int64
}

// renderArchive lists the files in an archive as a tree, directories
// suffixed with "/", drawn with box-drawing characters and each level
// indented by 2 spaces. Files are suffixed with their size in brackets.
func renderArchive(src archiveSource, w io.Writer) error {
	var entries []archiveEntry
	switch src.format {
	case "zip":
		zr, err := zip.NewReader(src.ra, src.size)
		if err != nil {
			return fmt.Errorf("%s: failed to read zip: %w", src.name, err)
		}
		for _, f := range zr.File {
			entries = append(entries, archiveEntry{name: f.Name, size: int64(f.UncompressedSize64)})
		}
	case "tar":
		tr := tar.NewReader(archiveTarInput(src.r))
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("%s: failed to read tar: %w", src.name, err)
			}
			entries = append(entries, archiveEntry{name: hdr.Name, size: hdr.Size})
		}
	default:
		return fmt.Errorf("%s: unrecognized archive format", src.name)
	}

	// Grey the box-drawing guides and darken file sizes when output is
	// styled, plain when piped; CATT_COLOR=yes/no overrides, same as the
	// other renderers.
	grey, dark, reset := "", "", ""
	if useDarkStyle() {
		grey, dark, reset = "\x1b[90m", "\x1b[38;5;243m", "\x1b[0m"
	}
	printTree(buildTree(entries), w, "", grey, dark, reset)
	return nil
}

// archiveSizeString renders a byte count in at most three digits plus
// a decimal unit suffix, e.g. "2B", "723B", "2.45MB", "123GB".
func archiveSizeString(size int64) string {
	if size < 1000 {
		return fmt.Sprintf("%dB", size)
	}
	units := []string{"KB", "MB", "GB", "TB", "PB", "EB"}
	v := float64(size)
	i := -1
	for v >= 1000 && i < len(units)-1 {
		v /= 1000
		i++
	}
	// Rounding can push the value back up to the next unit (999.5KB
	// displays as "1MB"); re-scale so at most three digits remain.
	if v >= 999.5 && i < len(units)-1 {
		v /= 1000
		i++
	}
	switch {
	case v >= 99.5:
		return fmt.Sprintf("%.0f%s", v, units[i])
	case v >= 9.95:
		return fmt.Sprintf("%.1f%s", v, units[i])
	default:
		return fmt.Sprintf("%.2f%s", v, units[i])
	}
}

// archiveTarInput wraps a tar-family stream, transparently
// decompressing gzip or bzip2 based on the leading bytes. Only a few
// bytes are peeked; the stream itself is never buffered in full.
func archiveTarInput(r io.Reader) io.Reader {
	br := bufio.NewReader(r)
	magic, _ := br.Peek(3)
	switch {
	case len(magic) >= 2 && bytes.Equal(magic[:2], []byte("\x1f\x8b")):
		if gz, err := gzip.NewReader(br); err == nil {
			return gz
		}
	case len(magic) >= 3 && bytes.Equal(magic[:3], []byte("BZh")):
		return bzip2.NewReader(br)
	}
	return br
}

// node is one entry in the archive tree. size is the uncompressed
// file size (0 for directories, which are never sized).
type node struct {
	name     string
	isDir    bool
	size     int64
	children map[string]*node
}

// buildTree turns flat archive entries into a nested tree. Names are
// cleaned of "./" prefixes and drive/absolute prefixes; directory
// entries are recorded as directories, and parent directories are
// created implicitly for deeper paths.
func buildTree(entries []archiveEntry) *node {
	root := &node{name: "", isDir: true, children: map[string]*node{}}
	for _, entry := range entries {
		e := strings.TrimPrefix(entry.name, "./")
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
			if i == len(parts)-1 && !wasDir {
				child.size = entry.size
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
// otherwise. Files are suffixed with their size in brackets, drawn in
// the darker grey.
func printTree(n *node, w io.Writer, prefix, grey, dark, reset string) {
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
			printTree(child, w, prefix+cont, grey, dark, reset)
		} else {
			size := fmt.Sprintf("(%s)", archiveSizeString(child.size))
			if dark != "" {
				size = dark + size + reset
			}
			fmt.Fprintf(w, "%s%s%s%s %s %s\n", grey, prefix, branch, reset, name, size)
		}
	}
}
