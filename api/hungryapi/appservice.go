package hungryapi

import (
	"context"
	"net/http"
	"net/url"
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

func NewClient(baseDomain, username, accessToken string) *Client {
	hungryURL := url.URL{
		Scheme: "https",
		Host:   "matrix." + baseDomain,
		Path:   "/_hungryserv/" + username,
	}
	client, err := mautrix.NewClient(hungryURL.String(), id.NewUserID(username, baseDomain), accessToken)
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
	ctx context.Context,
	bridge string,
	req ReqRegisterAppService,
) (resp appservice.Registration, err error) {
	url := cli.BuildURL(mautrix.BaseURLPath{"_matrix", "asmux", "mxauth", "appservice", cli.Username, bridge})
	_, err = cli.MakeRequest(ctx, http.MethodPut, url, &req, &resp)
	return
}

func (cli *Client) GetAppService(ctx context.Context, bridge string) (resp appservice.Registration, err error) {
	url := cli.BuildURL(mautrix.BaseURLPath{"_matrix", "asmux", "mxauth", "appservice", cli.Username, bridge})
	_, err = cli.MakeRequest(ctx, http.MethodGet, url, nil, &resp)
	return
}

func (cli *Client) DeleteAppService(ctx context.Context, bridge string) (err error) {
	url := cli.BuildURL(mautrix.BaseURLPath{"_matrix", "asmux", "mxauth", "appservice", cli.Username, bridge})
	_, err = cli.MakeRequest(ctx, http.MethodDelete, url, nil, nil)
	return
}

type respGetSystemTime struct {
	Time jsontime.UnixMilli `json:"time_ms"`
}

func (cli *Client) GetServerTime(ctx context.Context) (resp time.Time, precision time.Duration, err error) {
	var respData respGetSystemTime
	start := time.Now()
	_, err = cli.MakeFullRequest(ctx, mautrix.FullRequest{
		Method:       http.MethodGet,
		URL:          cli.BuildURL(mautrix.BaseURLPath{"_matrix", "client", "unstable", "com.beeper.timesync"}),
		ResponseJSON: &respData,
		MaxAttempts:  1,
	})
	precision = time.Since(start)
	resp = respData.Time.Time
	return
}
