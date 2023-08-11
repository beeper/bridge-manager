package hungryapi

import (
	"net/http"
	"time"

	"go.mau.fi/util/jsontime"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/appservice"
	"maunium.net/go/mautrix/id"
)

type Client struct {
	*mautrix.Client
	Username string
}

func NewClient(baseDomain, homeserverURL, username, accessToken string) *Client {
	client, err := mautrix.NewClient(homeserverURL, id.NewUserID(username, baseDomain), accessToken)
	if err != nil {
		panic(err)
	}
	return &Client{Client: client, Username: username}
}

type ReqRegisterAppService struct {
	Address    string `json:"address,omitempty"`
	Push       bool   `json:"push"`
	SelfHosted bool   `json:"self_hosted"`
}

func (cli *Client) RegisterAppService(
	bridge string,
	req ReqRegisterAppService,
) (resp appservice.Registration, err error) {
	url := cli.BuildURL(mautrix.BaseURLPath{"_matrix", "asmux", "mxauth", "appservice", cli.Username, bridge})
	_, err = cli.MakeRequest(http.MethodPut, url, &req, &resp)
	return
}

func (cli *Client) GetAppService(bridge string) (resp appservice.Registration, err error) {
	url := cli.BuildURL(mautrix.BaseURLPath{"_matrix", "asmux", "mxauth", "appservice", cli.Username, bridge})
	_, err = cli.MakeRequest(http.MethodGet, url, nil, &resp)
	return
}

func (cli *Client) DeleteAppService(bridge string) (err error) {
	url := cli.BuildURL(mautrix.BaseURLPath{"_matrix", "asmux", "mxauth", "appservice", cli.Username, bridge})
	_, err = cli.MakeRequest(http.MethodDelete, url, nil, nil)
	return
}

type respGetSystemTime struct {
	Time jsontime.UnixMilli `json:"time_ms"`
}

func (cli *Client) GetServerTime() (resp time.Time, precision time.Duration, err error) {
	var respData respGetSystemTime
	start := time.Now()
	_, err = cli.MakeFullRequest(mautrix.FullRequest{
		Method:       http.MethodGet,
		URL:          cli.BuildURL(mautrix.BaseURLPath{"_matrix", "client", "unstable", "com.beeper.timesync"}),
		ResponseJSON: &respData,
		MaxAttempts:  1,
	})
	precision = time.Since(start)
	resp = respData.Time.Time
	return
}
