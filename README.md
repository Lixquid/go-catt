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
- Displays PNG, JPG, and GIF files (using sixels on terminals that support them)
- Lists archives as a tree

## Usage

```
catt [file ...]
catt < file
catt -h | catt --help
```

Set `CATT_COLOR=yes` or `CATT_COLOR=no` to force color outputting on or off.
Otherwise, automatic detection will be used.

When output goes to a terminal and is larger than the viewport, it is piped
through a pager. The pager comes from the `PAGER` environment variable (which
may include arguments) and defaults to `less`. Set `CATT_PAGE=yes` to always
paginate, or `CATT_PAGE=no` to never paginate.

Set `CATT_MAX_ARCHIVE_SIZE` (e.g. `10MB`, `500KB`, or a plain byte count) to
skip archives that need decompressing (tgz, tar.gz, tbz, tar.bz2) or spooling
to disk (zip fed over stdin) when they exceed the limit. Plain tar and zip
files are read in place and never hit the limit.

## Build

```
go build -o catt .
```

## AI Disclaimer

This tool was created with AI.
