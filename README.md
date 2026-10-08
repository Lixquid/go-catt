<p align="center">
  <img src="misc/logo.svg" alt="catt logo" width="280">
</p>

<p align="center">
  <code>cat</code> for humans.
</p>

`catt` prints files with rendering or highlighting to make them easier to read
for humans, not tooling.

![A demo of catt](misc/demo.gif)

---

## Features

- Syntax highlights source code files
- Transforms markdown files into nicely rendered output
- Turns CSV files into tables
- Displays PNG, JPG, and GIF files (using the kitty graphics protocol or sixels on terminals that support them), recognized by extension or magic bytes, so extensionless and misnamed images still render (including from stdin)
- Lists archives as a tree
- Automatically pages large output to `$PAGER` (or `more` on Windows and `less` elsewhere if `$PAGER` isn't set)

## Usage

```
catt [file ...]
catt < file
catt -h | catt --help
```

Configuration is done via environment variables:
- `CATT_COLOR=yes|no` forces color output on or off. Otherwise, automatic
  detection of an interactive terminal will be used.
- `CATT_PAGE=yes|no` forces output to the pager. Otherwise, detection of if the
  viewport is large enough to contain it will be used.
- `CATT_MAX_ARCHIVE_SIZE=10MB|500KB` will skip listing archives above the given
  size if they're compressed tars (`tgz`, `tbz`, etc.) or zips from stdin to
  avoid making large files on disk. Plain tar and zip files on disk are read in
  place and do not trigger this limit. No value disables the limit.
- `CATT_IMAGE_PROTOCOL=auto|kitty|sixel|none` controls how images are drawn.
  `auto` (the default) detects the kitty graphics protocol first and falls back
  to sixels; `kitty` and `sixel` force a protocol; `none` passes image bytes
  through untouched. `CATT_KITTY=yes` is shorthand for
  `CATT_IMAGE_PROTOCOL=kitty`, and `CATT_KITTY=no` disables kitty detection so
  `auto` may fall back to sixels.

## Build

```
go build -o catt .
```

## AI Disclaimer

This tool was created with AI.
