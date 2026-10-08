package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"testing"
)

func TestArchiveFormatByExtension(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"x.zip", "zip"},
		{"x.ZIP", "zip"},
		{"x.tar", "tar"},
		{"x.tgz", "tar"},
		{"x.tar.gz", "tar"},
		{"x.tbz", "tar"},
		{"x.tbz2", "tar"},
		{"x.tar.bz2", "tar"},
		{"x.txt", ""},
		{"plain", ""},
		{"notatargz", ""}, // ".gz" alone is not an archive extension
	}
	for _, tt := range tests {
		if got := archiveFormatName(tt.name); got != tt.want {
			t.Errorf("archiveFormatName(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestArchiveFormatByMagic(t *testing.T) {
	// A misnamed file is still detected by its magic bytes.
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"weird.bin", []byte("PK\x03\x04rest"), "zip"},
		{"weird.bin", []byte("\x1f\x8bmore"), "tar"},
		{"weird.bin", []byte("BZh9stuff"), "tar"},
		{"weird.bin", append(make([]byte, 257), []byte("ustar")...), "tar"},
		{"weird.bin", []byte("plain text"), ""},
		{"weird.bin", nil, ""},
	}
	for _, tt := range tests {
		if got := archiveFormatHead(tt.data); got != tt.want {
			t.Errorf("archiveFormatHead(%q) = %q, want %q", tt.data, got, tt.want)
		}
	}
}

// renderArchiveBytes lists in-memory archive contents, mirroring the
// dispatch that catFile does for files (extension first, then magic
// bytes).
func renderArchiveBytes(name string, data []byte, w io.Writer) error {
	format := archiveFormatName(name)
	if format == "" {
		format = archiveFormatHead(data)
	}
	src := archiveSource{name: name, format: format}
	if format == "zip" {
		br := bytes.NewReader(data)
		src.ra, src.size = br, int64(len(data))
	} else {
		src.r = bytes.NewReader(data)
	}
	return renderArchive(src, w)
}

// buildTreeChildren returns the child node with the given name, failing
// the test if it does not exist.
func buildTreeChildren(t *testing.T, n *node, path ...string) *node {
	t.Helper()
	for _, p := range path {
		child, ok := n.children[p]
		if !ok {
			t.Fatalf("no child %q under %q (children: %v)", p, n.name, n.children)
		}
		n = child
	}
	return n
}

func TestBuildTreeBasic(t *testing.T) {
	root := buildTree([]string{"dir/file.txt", "other.txt"})
	f := buildTreeChildren(t, root, "dir", "file.txt")
	if f.isDir {
		t.Error("dir/file.txt should not be a directory")
	}
	// The parent directory is created implicitly and marked as a dir.
	if !buildTreeChildren(t, root, "dir").isDir {
		t.Error("implicit parent dir should be a directory")
	}
	other := buildTreeChildren(t, root, "other.txt")
	if other.isDir {
		t.Error("other.txt should not be a directory")
	}
}

// TestBuildTreeDirEntry is a regression test: directory entries
// ("name/") used to be recorded as files because the trailing "/" was
// trimmed before the directory check ran.
func TestBuildTreeDirEntry(t *testing.T) {
	root := buildTree([]string{"src/"})
	if !buildTreeChildren(t, root, "src").isDir {
		t.Error(`entry "src/" should be marked as a directory`)
	}
}

func TestBuildTreePrefixes(t *testing.T) {
	// "./" and leading "/" prefixes are stripped, and ".." components
	// are dropped entirely.
	root := buildTree([]string{"./x.txt", "/y.txt", "../etc/passwd", "./"})
	buildTreeChildren(t, root, "x.txt")
	buildTreeChildren(t, root, "y.txt")
	if len(root.children) != 2 {
		t.Errorf("expected 2 children after cleaning, got %v", root.children)
	}
}

// TestBuildTreeDrivePrefix is a regression test: the Windows drive
// prefix used to be stripped from any name whose second character was
// ":", mangling names like "a:b.txt"; now a real "X:/" (or "X:\\")
// drive prefix is required.
func TestBuildTreeDrivePrefix(t *testing.T) {
	root := buildTree([]string{"C:/Users/file.txt", "a:b.txt", "12:30/x"})
	buildTreeChildren(t, root, "Users", "file.txt")
	buildTreeChildren(t, root, "a:b.txt")
	buildTreeChildren(t, root, "12:30", "x")
	if len(root.children) != 3 {
		t.Errorf("unexpected children: %v", root.children)
	}
}

func TestIsASCIIAlpha(t *testing.T) {
	for _, c := range []byte{'a', 'z', 'A', 'Z'} {
		if !isASCIIAlpha(c) {
			t.Errorf("isASCIIAlpha(%q) = false, want true", c)
		}
	}
	for _, c := range []byte{'0', ':', '_', ' ', '\xc4'} {
		if isASCIIAlpha(c) {
			t.Errorf("isASCIIAlpha(%q) = true, want false", c)
		}
	}
}

// makeTestZip builds an in-memory zip with a directory entry and a file.
func makeTestZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if _, err := zw.Create("dir/"); err != nil {
		t.Fatal(err)
	}
	f, err := zw.Create("dir/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// makeTestTgz builds an in-memory gzipped tar with a directory entry
// and a file.
func makeTestTgz(t *testing.T) []byte {
	t.Helper()
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	if err := tw.WriteHeader(&tar.Header{Name: "dir/", Typeflag: tar.TypeDir}); err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: "dir/hello.txt", Size: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(raw.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestRenderArchive(t *testing.T) {
	t.Setenv("CATT_COLOR", "no")
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"x.zip", makeTestZip(t), "└─ dir/\n   └─ hello.txt\n"},
		{"x.tgz", makeTestTgz(t), "└─ dir/\n   └─ hello.txt\n"},
		// Magic bytes win over the (non-archive) extension.
		{"x.bin", makeTestZip(t), "└─ dir/\n   └─ hello.txt\n"},
	}
	for _, tt := range tests {
		var buf bytes.Buffer
		if err := renderArchiveBytes(tt.name, tt.data, &buf); err != nil {
			t.Fatalf("renderArchiveBytes(%q): %v", tt.name, err)
		}
		if got := buf.String(); got != tt.want {
			t.Errorf("renderArchiveBytes(%q) tree = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestRenderArchiveUnrecognized(t *testing.T) {
	var buf bytes.Buffer
	if err := renderArchiveBytes("x.zip", []byte("not an archive"), &buf); err == nil {
		t.Error("renderArchiveBytes with invalid zip data should fail")
	}
}

func TestPrintTreeOrdering(t *testing.T) {
	// Directories come before files, both case-insensitively sorted.
	root := buildTree([]string{"Beta.txt", "apple/", "alpha.txt"})
	var buf bytes.Buffer
	printTree(root, &buf, "", "", "")
	want := "├─ apple/\n├─ alpha.txt\n└─ Beta.txt\n"
	if got := buf.String(); got != want {
		t.Errorf("printTree = %q, want %q", got, want)
	}
}

func TestPrintTreeGuides(t *testing.T) {
	// Directories that have following siblings draw a vertical guide
	// under them; the last one gets blank padding.
	root := buildTree([]string{"a/1.txt", "b/2.txt", "c.txt"})
	var buf bytes.Buffer
	printTree(root, &buf, "", "", "")
	want := "├─ a/\n│  └─ 1.txt\n├─ b/\n│  └─ 2.txt\n└─ c.txt\n"
	if got := buf.String(); got != want {
		t.Errorf("printTree = %q, want %q", got, want)
	}
}

func TestPrintTreeGreyGuides(t *testing.T) {
	// When styling is enabled the box-drawing guides are wrapped in
	// grey ANSI codes, the names are not.
	root := buildTree([]string{"x.txt"})
	var buf bytes.Buffer
	printTree(root, &buf, "", "\x1b[90m", "\x1b[0m")
	want := "\x1b[90m└─\x1b[0m x.txt\n"
	if got := buf.String(); got != want {
		t.Errorf("printTree = %q, want %q", got, want)
	}
}

func TestRenderStreamArchiveDispatch(t *testing.T) {
	t.Setenv("CATT_COLOR", "no")
	// A zip stream is listed as a tree even when it has no name hint:
	// the sniffed prefix is enough. The spooled temp-file path for zip
	// is exercised too, since stdin is not seekable.
	data := makeTestZip(t)
	split := min(archiveHeadSize, len(data))
	var buf bytes.Buffer
	if err := renderStream("whatever.bin", data[:split], bytes.NewReader(data[split:]), &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "hello.txt") {
		t.Errorf("zip data should be rendered as a tree, got %q", buf.String())
	}
}
