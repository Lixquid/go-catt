package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
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
	root := buildTree([]archiveEntry{
		{"dir/file.txt", 2048},
		{"other.txt", 7},
	})
	f := buildTreeChildren(t, root, "dir", "file.txt")
	if f.isDir {
		t.Error("dir/file.txt should not be a directory")
	}
	if f.size != 2048 {
		t.Errorf("dir/file.txt size = %d, want 2048", f.size)
	}
	// The parent directory is created implicitly and marked as a dir.
	if !buildTreeChildren(t, root, "dir").isDir {
		t.Error("implicit parent dir should be a directory")
	}
	other := buildTreeChildren(t, root, "other.txt")
	if other.isDir {
		t.Error("other.txt should not be a directory")
	}
	if other.size != 7 {
		t.Errorf("other.txt size = %d, want 7", other.size)
	}
}

// TestBuildTreeDirEntry is a regression test: directory entries
// ("name/") used to be recorded as files because the trailing "/" was
// trimmed before the directory check ran.
func TestBuildTreeDirEntry(t *testing.T) {
	root := buildTree([]archiveEntry{{"src/", 0}})
	if !buildTreeChildren(t, root, "src").isDir {
		t.Error(`entry "src/" should be marked as a directory`)
	}
}

func TestBuildTreePrefixes(t *testing.T) {
	// "./" and leading "/" prefixes are stripped, and ".." components
	// are dropped entirely.
	root := buildTree([]archiveEntry{{"./x.txt", 1}, {"/y.txt", 2}, {"../etc/passwd", 3}, {"./", 0}})
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
	root := buildTree([]archiveEntry{{"C:/Users/file.txt", 1}, {"a:b.txt", 2}, {"12:30/x", 3}})
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
		{"x.zip", makeTestZip(t), "└─ dir/\n   └─ hello.txt (2B)\n"},
		{"x.tgz", makeTestTgz(t), "└─ dir/\n   └─ hello.txt (2B)\n"},
		// Magic bytes win over the (non-archive) extension.
		{"x.bin", makeTestZip(t), "└─ dir/\n   └─ hello.txt (2B)\n"},
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

func TestArchiveSizeString(t *testing.T) {
	// Byte counts render with at most three digits plus a unit suffix.
	tests := []struct {
		size int64
		want string
	}{
		{0, "0B"},
		{2, "2B"},
		{723, "723B"},
		{999, "999B"},
		{1000, "1.00KB"},
		{2560, "2.56KB"},
		{2560000, "2.56MB"},
		{123400000, "123MB"},
		{2560000000, "2.56GB"},
		{1099511627776, "1.10TB"},
	}
	for _, tt := range tests {
		if got := archiveSizeString(tt.size); got != tt.want {
			t.Errorf("archiveSizeString(%d) = %q, want %q", tt.size, got, tt.want)
		}
	}
}

func TestRenderArchiveStyledSizes(t *testing.T) {
	// With styling enabled the size is wrapped in darker grey codes;
	// zip sizes come from the central directory (UncompressedSize64).
	t.Setenv("CATT_COLOR", "yes")
	var buf bytes.Buffer
	if err := renderArchiveBytes("x.zip", makeTestZip(t), &buf); err != nil {
		t.Fatal(err)
	}
	want := "\x1b[90m└─\x1b[0m dir/\n\x1b[90m   └─\x1b[0m hello.txt \x1b[38;5;238m(2B)\x1b[0m\n"
	if got := buf.String(); got != want {
		t.Errorf("styled tree = %q, want %q", got, want)
	}
}

func TestPrintTreeOrdering(t *testing.T) {
	// Directories come before files, both case-insensitively sorted.
	root := buildTree([]archiveEntry{{"Beta.txt", 0}, {"apple/", 0}, {"alpha.txt", 0}})
	var buf bytes.Buffer
	printTree(root, &buf, "", "", "", "")
	want := "├─ apple/\n├─ alpha.txt (0B)\n└─ Beta.txt (0B)\n"
	if got := buf.String(); got != want {
		t.Errorf("printTree = %q, want %q", got, want)
	}
}

func TestPrintTreeGuides(t *testing.T) {
	// Directories that have following siblings draw a vertical guide
	// under them; the last one gets blank padding.
	root := buildTree([]archiveEntry{{"a/1.txt", 0}, {"b/2.txt", 0}, {"c.txt", 0}})
	var buf bytes.Buffer
	printTree(root, &buf, "", "", "", "")
	want := "├─ a/\n│  └─ 1.txt (0B)\n├─ b/\n│  └─ 2.txt (0B)\n└─ c.txt (0B)\n"
	if got := buf.String(); got != want {
		t.Errorf("printTree = %q, want %q", got, want)
	}
}

func TestPrintTreeGreyGuides(t *testing.T) {
	// When styling is enabled the box-drawing guides are wrapped in
	// grey ANSI codes, the names are not.
	root := buildTree([]archiveEntry{{"x.txt", 0}})
	var buf bytes.Buffer
	printTree(root, &buf, "", "\x1b[90m", "\x1b[38;5;238m", "\x1b[0m")
	want := "\x1b[90m└─\x1b[0m x.txt \x1b[38;5;238m(0B)\x1b[0m\n"
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

// countReader counts the bytes consumed from the underlying reader.
type countReader struct {
	r io.Reader
	n int64
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func TestParseArchiveLimit(t *testing.T) {
	tests := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"", 0, true},
		{"  ", 0, true},
		{"0", 0, true},
		{"1024", 1024, true},
		{"10KB", 10000, true},
		{"10kb", 10000, true},
		{"10k", 10000, true},
		{"1.5MB", 1500000, true},
		{"2GB", 2000000000, true},
		{"1TB", 1000000000000, true},
		{"500B", 500, true},
		{"  500KB  ", 500000, true},
		{"abc", 0, false},
		{"-5", 0, false},
		{"1.2.3KB", 0, false},
	}
	for _, tt := range tests {
		got, err := parseArchiveLimit(tt.in)
		if tt.ok {
			if err != nil {
				t.Errorf("parseArchiveLimit(%q) unexpected error: %v", tt.in, err)
				continue
			}
			if got != tt.want {
				t.Errorf("parseArchiveLimit(%q) = %d, want %d", tt.in, got, tt.want)
			}
		} else if err == nil {
			t.Errorf("parseArchiveLimit(%q) should fail, got %d", tt.in, got)
		}
	}
}

// makeBigTgz builds a gzipped tar holding a single file of n
// pseudo-random (incompressible) bytes, so the archive size can be
// controlled precisely.
func makeBigTgz(t *testing.T, n int) []byte {
	t.Helper()
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	if err := tw.WriteHeader(&tar.Header{Name: "big.bin", Size: int64(n)}); err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, n)
	if _, err := rand.New(rand.NewSource(1)).Read(payload); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(payload); err != nil {
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

func TestRenderArchiveFileTooLarge(t *testing.T) {
	t.Setenv("CATT_COLOR", "no")
	t.Setenv(envArchiveLimit, "100B")
	data := makeBigTgz(t, 5000)
	path := filepath.Join(t.TempDir(), "big.tgz")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var buf bytes.Buffer
	if err := renderArchiveFile(path, f, "tar", &buf); err != nil {
		t.Fatalf("renderArchiveFile: %v", err)
	}
	want := fmt.Sprintf("└─ %s (%s)\n   file is too large to scan\n", path, archiveSizeString(int64(len(data))))
	if got := buf.String(); got != want {
		t.Errorf("too-large notice = %q, want %q", got, want)
	}
}

func TestRenderArchiveFileTooLargeStyled(t *testing.T) {
	t.Setenv("CATT_COLOR", "yes")
	t.Setenv(envArchiveLimit, "100B")
	data := makeBigTgz(t, 5000)
	path := filepath.Join(t.TempDir(), "big.tgz")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var buf bytes.Buffer
	if err := renderArchiveFile(path, f, "tar", &buf); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("\x1b[90m└─\x1b[0m %s \x1b[38;5;238m(%s)\x1b[0m\n   file is too large to scan\n", path, archiveSizeString(int64(len(data))))
	if got := buf.String(); got != want {
		t.Errorf("styled too-large notice = %q, want %q", got, want)
	}
}

func TestRenderArchiveFileUnderLimit(t *testing.T) {
	t.Setenv("CATT_COLOR", "no")
	t.Setenv(envArchiveLimit, "10MB")
	path := filepath.Join(t.TempDir(), "ok.tgz")
	data := makeBigTgz(t, 5000)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var buf bytes.Buffer
	if err := renderArchiveFile(path, f, "tar", &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "big.bin") {
		t.Errorf("archive under the limit should be listed, got %q", buf.String())
	}
}

func TestRenderArchiveFileNoLimitForPlainTarAndZip(t *testing.T) {
	// Plain tar and zip files are read in place without decompressing
	// or spooling, so the size limit does not apply to them.
	t.Setenv("CATT_COLOR", "no")
	t.Setenv(envArchiveLimit, "10B")

	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	if err := tw.WriteHeader(&tar.Header{Name: "big.bin", Size: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	zipData := makeTestZip(t)

	tests := []struct {
		name string
		data []byte
		frag string
	}{
		{"plain.tar", raw.Bytes(), "big.bin"},
		{"plain.zip", zipData, "hello.txt"},
	}
	for _, tt := range tests {
		path := filepath.Join(t.TempDir(), tt.name)
		if err := os.WriteFile(path, tt.data, 0o644); err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := renderArchiveFile(path, f, archiveFormatName(path), &buf); err != nil {
			t.Errorf("renderArchiveFile(%q): %v", tt.name, err)
		} else if !strings.Contains(buf.String(), tt.frag) {
			t.Errorf("%s over the limit should still be listed, got %q", tt.name, buf.String())
		}
		f.Close()
	}
}

func TestRenderArchiveStreamZipTooLarge(t *testing.T) {
	// A zip stream is spooled to disk, so the limit applies: reading
	// must stop as soon as the limit is exceeded and only the error is
	// reported.
	t.Setenv("CATT_COLOR", "no")
	t.Setenv(envArchiveLimit, "100B")
	data := makeTestZip(t)
	split := min(archiveHeadSize, len(data))
	cr := &countReader{r: bytes.NewReader(data[split:])}
	var buf bytes.Buffer
	err := renderStream("<stdin>", data[:split], cr, &buf)
	if err == nil {
		t.Fatal("oversized zip stream should fail")
	}
	if !errors.Is(err, errArchiveTooLarge) {
		t.Errorf("error should be errArchiveTooLarge, got %v", err)
	}
	if !strings.Contains(err.Error(), "limit 100B") {
		t.Errorf("error should mention the limit, got %q", err.Error())
	}
	if cr.n > 100 {
		t.Errorf("reading should stop near the limit, read %d bytes", cr.n)
	}
	if buf.Len() != 0 {
		t.Errorf("nothing should be printed for a too-large stream, got %q", buf.String())
	}
}

func TestRenderArchiveStreamCompressedTarTooLarge(t *testing.T) {
	// A compressed tar stream needs decompression, so the limit
	// applies to the raw bytes read.
	t.Setenv("CATT_COLOR", "no")
	t.Setenv(envArchiveLimit, "100B")
	data := makeBigTgz(t, 5000)
	split := min(archiveHeadSize, len(data))
	cr := &countReader{r: bytes.NewReader(data[split:])}
	var buf bytes.Buffer
	err := renderStream("<stdin>", data[:split], cr, &buf)
	if err == nil {
		t.Fatal("oversized tgz stream should fail")
	}
	if !errors.Is(err, errArchiveTooLarge) {
		t.Errorf("error should be errArchiveTooLarge, got %v", err)
	}
	if cr.n > 100 {
		t.Errorf("reading should stop near the limit, read %d bytes", cr.n)
	}
}

func TestRenderArchiveStreamUnderLimit(t *testing.T) {
	// Streams within the limit are listed normally.
	t.Setenv("CATT_COLOR", "no")
	t.Setenv(envArchiveLimit, "10MB")
	data := makeBigTgz(t, 5000)
	split := min(archiveHeadSize, len(data))
	var buf bytes.Buffer
	if err := renderStream("<stdin>", data[:split], bytes.NewReader(data[split:]), &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "big.bin") {
		t.Errorf("stream under the limit should be listed, got %q", buf.String())
	}
}

func TestRenderArchiveStreamPlainTarNoLimit(t *testing.T) {
	// A plain tar stream is neither decompressed nor spooled, so the
	// limit does not apply.
	t.Setenv("CATT_COLOR", "no")
	t.Setenv(envArchiveLimit, "10B")
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	if err := tw.WriteHeader(&tar.Header{Name: "big.bin", Size: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	split := min(archiveHeadSize, raw.Len())
	var buf bytes.Buffer
	if err := renderStream("<stdin>", raw.Bytes()[:split], bytes.NewReader(raw.Bytes()[split:]), &buf); err != nil {
		t.Errorf("plain tar over the limit should still be listed: %v", err)
	} else if !strings.Contains(buf.String(), "big.bin") {
		t.Errorf("plain tar over the limit should be listed, got %q", buf.String())
	}
}

func TestRenderArchiveInvalidLimit(t *testing.T) {
	t.Setenv("CATT_COLOR", "no")
	t.Setenv(envArchiveLimit, "nonsense")
	data := makeTestZip(t)
	split := min(archiveHeadSize, len(data))
	var buf bytes.Buffer
	err := renderStream("<stdin>", data[:split], bytes.NewReader(data[split:]), &buf)
	if err == nil || !strings.Contains(err.Error(), "CATT_MAX_ARCHIVE_SIZE") {
		t.Errorf("invalid limit should fail with a mention of the variable, got %v", err)
	}
}
