package migration

import (
	"fmt"
	"os"
)

const (
	red    = "\033[31m"
	yellow = "\033[33m"
	purple = "\033[35m"
	bold   = "\033[1m"
	reset  = "\033[0m"
	green  = "\033[32m"
)

// Ld function is a part of logging. Ld
func ld(msg string) {
	if Debug {
		fmt.Printf("%s: %s\n",
			colorize("DEBUG", yellow+bold),
			colorize(msg, yellow),
		)
	}
}

// Lw function is a part of logging. Lw
func lw(msg string) {
	fmt.Printf("%s: %s\n",
		colorize("WARNING", purple),
		colorize(msg, ""),
	)
}

// Le function is a part of logging. Le
func le(msg string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n",
		colorize("ERROR", red+bold),
		colorize(msg, red),
	)
}

// Colorize function
func colorize(s, color string) string {
	if NoColor {
		return s
	}
	return color + s + reset
}
