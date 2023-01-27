# go-latest

`go-latest` installs the latest version of programs (packages) installed by `go install`.

`go-latest` skips programs that have versions like `(devel)` or a specific SHA attached.

Only works on programs installed after Go started adding version info to binaries.
Check through `go version -m $(go env GOBIN)/foo`.

NOTE: This program will update programs in `GOBIN`, run at your own risk.

## Install

```
go install github.com/vikblom/go-latest@latest
```

## Usage
```
# Inspect
go-latest -h
go-latest -v

# Update programs
go-latest
```
