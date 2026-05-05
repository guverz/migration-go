package migration

import (
	"fmt"
	"os"
)

const (
	Red    = "\033[31m"
	Yellow = "\033[33m"
	Purple = "\033[35m"
	Bold   = "\033[1m"
	Reset  = "\033[0m"
	Green  = "\033[32m"
)

// Ld function is a part of logging. Ld
func Ld(msg string) {
	if Debug {
		fmt.Printf("%s: %s\n",
			Colorize("DEBUG", Yellow+Bold),
			Colorize(msg, Yellow),
		)
	}
}

// Lw function is a part of logging. Lw
func Lw(msg string) {
	fmt.Printf("%s: %s\n",
		Colorize("WARNING", Purple),
		Colorize(msg, ""),
	)
}

// Le function is a part of logging. Le
func Le(msg string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n",
		Colorize("ERROR", Red+Bold),
		Colorize(msg, Red),
	)
}

// Colorize function
func Colorize(s, color string) string {
	if NoColor {
		return s
	}
	return color + s + Reset
}
