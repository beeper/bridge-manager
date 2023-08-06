package bridgeservice

import (
	_ "embed"
	"encoding/xml"
	"fmt"
	"strings"
	"text/template"
)

//go:embed systemd.tpl.service
var systemdServiceRaw string
var systemdService *template.Template

//go:embed launchagent.tpl.plist
var launchAgentRaw string
var launchAgent *template.Template

var tplFuncs = map[string]any{
	"xmlescape": func(input string) string {
		var buf strings.Builder
		_ = xml.EscapeText(&buf, []byte(input))
		return buf.String()
	},
	"shelljoin": func(input []string) string {
		var buf strings.Builder
		for _, arg := range input {
			// TODO use proper shell quoting?
			_, _ = fmt.Fprintf(&buf, "%q ", arg)
		}
		return buf.String()[:buf.Len()-1]
	},
	"join": strings.Join,
}

func init() {
	var err error
	systemdService, err = template.New("systemdService").Funcs(tplFuncs).Parse(systemdServiceRaw)
	if err != nil {
		panic(fmt.Errorf("failed to parse systemd service template: %w", err))
	}
	launchAgent, err = template.New("launchAgent").Funcs(tplFuncs).Parse(launchAgentRaw)
	if err != nil {
		panic(fmt.Errorf("failed to parse LaunchAgent template: %w", err))
	}
}

type Params struct {
	BridgeType string
	Name       string
	WorkDir    string
}

func Systemd(params Params) (string, error) {
	var out strings.Builder
	err := systemdService.Execute(&out, &params)
	return out.String(), err
}

func LaunchAgent(params Params) (string, error) {
	var out strings.Builder
	err := launchAgent.Execute(&out, &params)
	return out.String(), err
}
