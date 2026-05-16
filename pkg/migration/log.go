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

// ld function is a part of logging.
func ld(msg string) {
	if Debug {
		fmt.Printf("%s: %s\n",
			colorize("DEBUG", yellow+bold),
			colorize(msg, yellow),
		)
	}
}

// lw function is a part of logging. lw
func lw(msg string) {
	fmt.Printf("%s: %s\n",
		colorize("WARNING", purple),
		colorize(msg, ""),
	)
}

// le function is a part of logging. le
func le(msg string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n",
		colorize("ERROR", red+bold),
		colorize(msg, red),
	)
}

// colorize function
func colorize(s, color string) string {
	if NoColor {
		return s
	}
	return color + s + reset
}
