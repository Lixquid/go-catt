# catt

`catt` (cat themed) is `cat(1)` with pretty outputs.

## Features

- Renders markdown into pretty formatted output
- Formats CSV files as tables, identical in appearance to tables from
  markdown files
- Pretty-prints code files with syntax highlighting
- Displays PNG files as sixel graphics on terminals that support them
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
