package hungryapi

import (
	"fmt"
	"net/http"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/appservice"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/util/jsontime"
)

type Client struct {
	*mautrix.Client
	Username string
}

const HungryURLTemplate = "https://lb.nodes.%s.bridges.%s/%s"
const HungryDirectURLTemplate = "https://%s.nodes.%s.bridges.%s/%s/"

func NewClient(baseDomain, clusterID, username, accessToken string) *Client {
	homeserverURL := fmt.Sprintf(HungryURLTemplate, clusterID, baseDomain, username)
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
