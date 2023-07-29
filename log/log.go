package log

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/mitchellh/colorstring"
)

func Printf(format string, args ...any) {
	if !color.NoColor {
		format = colorstring.Color(format)
	}
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
}
