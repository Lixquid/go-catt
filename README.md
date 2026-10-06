# catt

`catt` (cat themed) is `cat(1)` with pretty outputs.

## Features

- Renders markdown into pretty formatted output
- Pretty-prints code files with syntax highlighting (via
  [chroma](https://github.com/alecthomas/chroma)); unknown file types
  are printed as-is
- Style selection: markdown and code files always share the same
  styling - `dark` on a TTY, `ascii`/plain when piped
- Set `CATT_COLOR=yes` to force dark styling even when piped, or
  `CATT_COLOR=no` to force plain output even on a terminal

## Usage

```
catt [file ...]
catt < file
catt -h | catt --help
```

## Build

```
go build -o catt .
```
