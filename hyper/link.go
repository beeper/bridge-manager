package hyper

import (
	"fmt"
	"os"
)

var Disable = os.Getenv("TERM") == "dumb"

const OSC = "\x1b]"
const OSC8 = OSC + "8"
const ST = "\x07" // or "\x1b\\"
const URLTemplate = OSC8 + ";%s;%s" + ST + "%s" + OSC8 + ";;" + ST

func Link(text string, url string, important bool) string {
	if Disable {
		if !important {
			return text
		}
		return fmt.Sprintf("%s (%s)", text, url)
	}
	params := ""
	return fmt.Sprintf(URLTemplate, params, url, text)
}
