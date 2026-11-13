package main

import (
	_ "embed"
)

//go:embed update.sh
var embeddedUpdateScript []byte
