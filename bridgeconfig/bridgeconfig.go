package bridgeconfig

import (
	"embed"
	"fmt"
	"reflect"
	"strings"
	"text/template"

	"maunium.net/go/mautrix/id"
)

type BridgeV2Name struct {
	DatabaseFileName string
	CommandPrefix    string
	BridgeTypeName   string
	BridgeTypeIcon   string
	DefaultPickleKey string

	MaxInitialMessages  int
	MaxBackwardMessages int
}

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
	Username     string
	UserID       id.UserID

	ProvisioningSecret string

	DatabasePrefix string

	BridgeV2Name

	Params map[string]string
}

//go:embed *.tpl.yaml
var configs embed.FS
var tpl *template.Template
var SupportedBridges []string

var tplFuncs = template.FuncMap{
	"replace": strings.ReplaceAll,
	"setfield": func(obj any, field string, value any) any {
		val := reflect.ValueOf(obj)
		for val.Kind() == reflect.Pointer {
			val = val.Elem()
		}
		val.FieldByName(field).Set(reflect.ValueOf(value))
		return ""
	},
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

func templateName(bridgeName string) string {
	return fmt.Sprintf("%s.tpl.yaml", bridgeName)
}

func IsSupported(bridgeName string) bool {
	return tpl.Lookup(templateName(bridgeName)) != nil
}

func Generate(bridgeName string, params Params) (string, error) {
	var out strings.Builder
	err := tpl.ExecuteTemplate(&out, templateName(bridgeName), &params)
	return out.String(), err
}
