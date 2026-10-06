# catt

`catt` (cat themed) is `cat(1)` with pretty outputs.

## Features

- Renders markdown into pretty formatted output
- Prints any other file as-is
- Style selection: `dark` on a TTY, `ascii` when piped - override with
  `CATT_STYLE` (e.g. `CATT_STYLE=dracula catt README.md`)

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
