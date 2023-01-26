package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/urfave/cli/v2"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/appservice"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/util/jsontime"
)

func main() {
	app := &cli.App{
		Name:  "bbctl",
		Usage: "Manage self-hosted bridges for Beeper",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "server",
				Aliases: []string{"s"},
				Usage:   "Use `SERVER` as the homeserver",
			},
			&cli.StringFlag{
				Name:    "username",
				Aliases: []string{"u"},
				Usage:   "`USERNAME` to authenticate as",
			},
			&cli.StringFlag{
				Name:    "token",
				Aliases: []string{"t"},
				Usage:   "`ACCESS_TOKEN` from the Beeper app",
			},
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

type HungryAPI struct {
	*mautrix.Client
	Username string
}

const MatrixURLTemplate = "https://matrix.%s"

func NewMatrixAPI(baseDomain, username, accessToken string) *mautrix.Client {
	homeserverURL := fmt.Sprintf(MatrixURLTemplate, baseDomain)
	var userID id.UserID
	if username != "" {
		userID = id.NewUserID(username, baseDomain)
	}
	client, err := mautrix.NewClient(homeserverURL, userID, accessToken)
	if err != nil {
		panic(err)
	}
	return client
}

const HungryURLTemplate = "https://%s.users.%s.bridges.%s/hungryserv"

func NewHungryAPI(baseDomain, clusterID, username, accessToken string) *HungryAPI {
	homeserverURL := fmt.Sprintf(HungryURLTemplate, username, clusterID, baseDomain)
	client, err := mautrix.NewClient(homeserverURL, id.NewUserID(username, baseDomain), accessToken)
	if err != nil {
		panic(err)
	}
	return &HungryAPI{Client: client, Username: username}
}

type ReqRegisterAppService struct {
	Address string `json:"address,omitempty"`
	Push    bool   `json:"push,omitempty"`
}

func (hapi *HungryAPI) RegisterAppService(
	bridge string,
	req ReqRegisterAppService,
) (resp appservice.Registration, err error) {
	url := hapi.BuildURL(mautrix.BaseURLPath{"_matrix", "asmux", "mxauth", "appservice", hapi.Username, bridge})
	_, err = hapi.MakeRequest(http.MethodPut, url, &req, &resp)
	return
}

func (hapi *HungryAPI) GetAppService(bridge string) (resp appservice.Registration, err error) {
	url := hapi.BuildURL(mautrix.BaseURLPath{"_matrix", "asmux", "mxauth", "appservice", hapi.Username, bridge})
	_, err = hapi.MakeRequest(http.MethodGet, url, nil, &resp)
	return
}

func (hapi *HungryAPI) DeleteAppService(bridge string) (err error) {
	url := hapi.BuildURL(mautrix.BaseURLPath{"_matrix", "asmux", "mxauth", "appservice", hapi.Username, bridge})
	_, err = hapi.MakeRequest(http.MethodDelete, url, nil, nil)
	return
}

type respGetSystemTime struct {
	Time jsontime.UnixMilli `json:"time_ms"`
}

func (hapi *HungryAPI) GetServerTime() (resp time.Time, precision time.Duration, err error) {
	var respData respGetSystemTime
	start := time.Now()
	_, err = hapi.MakeFullRequest(mautrix.FullRequest{
		Method:       http.MethodGet,
		URL:          hapi.BuildURL(mautrix.BaseURLPath{"_matrix", "client", "unstable", "com.beeper.timesync"}),
		ResponseJSON: &respData,
		MaxAttempts:  1,
	})
	precision = time.Since(start)
	resp = respData.Time.Time
	return
}
