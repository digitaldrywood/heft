package config

import _ "embed"

//go:embed defaults/golangci.yml
var DefaultGolangCI []byte

//go:embed defaults/custom-gcl.yml
var DefaultCustomGCL []byte
