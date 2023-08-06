package bridgeconfig

import (
	"embed"
	"fmt"
	"strings"
	"text/template"

	"maunium.net/go/mautrix/id"
)

type Params struct {
	HungryAddress string
	BeeperDomain  string

	Websocket  bool
	ListenAddr string
	ListenPort uint16

	AppserviceID string
	ASToken      string
	HSToken      string
	BridgeName   string
	UserID       id.UserID

	DatabasePrefix string

	Params map[string]string
}

//go:embed *.tpl.yaml
var configs embed.FS
var tpl *template.Template
var SupportedBridges []string

var tplFuncs = map[string]any{
	"replace": strings.ReplaceAll,
}

func init() {
	var err error
	tpl, err = template.New("configs").Funcs(tplFuncs).ParseFS(configs, "*")
	if err != nil {
		panic(fmt.Errorf("failed to parse bridge config templates: %w", err))
	}
	for _, sub := range tpl.Templates() {
		SupportedBridges = append(SupportedBridges, strings.TrimSuffix(sub.Name(), ".tpl.yaml"))
	}
}

func templateName(bridgeType string) string {
	return fmt.Sprintf("%s.tpl.yaml", bridgeType)
}

func IsSupported(bridgeType string) bool {
	return tpl.Lookup(templateName(bridgeType)) != nil
}

func Generate(bridgeType string, params Params) (string, error) {
	var out strings.Builder
	err := tpl.ExecuteTemplate(&out, templateName(bridgeType), &params)
	return out.String(), err
}
