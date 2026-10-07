<p align="center">
  <img src="misc/logo.svg" alt="catt logo" width="280">
</p>

<h1 align="center">catt</h1>

<p align="center">
  <code>cat</code> for humans.
</p>

---

`catt` prints files with rendering or highlighting to make them easier to read
for humans, not tooling.

## Features

- Syntax highlights source code files
- Transforms markdown files into nicely rendered output
- Turns CSV files into tables
- Displays PNG, JPG, and GIF files (using sixels on terminals that support
  them)
- Lists archives as a tree

## Usage

```
catt [file ...]
catt < file
catt -h | catt --help
```

Set `CATT_COLOR=yes` or `CATT_COLOR=no` to force color outputting on or off.
Otherwise, automatic detection will be used.

## Build

```
go build -o catt .
```
