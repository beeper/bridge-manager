package beeperapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridge/status"
	"maunium.net/go/mautrix/id"
)

type BridgeState struct {
	Username   string                  `json:"username"`
	Bridge     string                  `json:"bridge"`
	StateEvent status.BridgeStateEvent `json:"stateEvent"`
	Source     string                  `json:"source"`
	CreatedAt  time.Time               `json:"createdAt"`
	Reason     string                  `json:"reason"`
	Info       map[string]any          `json:"info"`
}

type WhoamiBridge struct {
	Version       string `json:"version"`
	ConfigHash    string `json:"configHash"`
	OtherVersions []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"otherVersions"`
	BridgeState BridgeState                   `json:"bridgeState"`
	RemoteState map[string]status.BridgeState `json:"remoteState"`
}

type WhoamiAsmuxData struct {
	ID         string `json:"id"`
	APIToken   string `json:"api_token"`
	LoginToken string `json:"login_token"`
}

type WhoamiUser struct {
	Bridges    map[string]WhoamiBridge `json:"bridges"`
	Hungryserv WhoamiBridge            `json:"hungryserv"`
	AsmuxData  WhoamiAsmuxData         `json:"asmuxData"`
}

type WhoamiUserInfo struct {
	CreatedAt           time.Time `json:"createdAt"`
	Username            string    `json:"username"`
	Email               string    `json:"email"`
	FullName            string    `json:"fullName"`
	Channel             string    `json:"channel"`
	Admin               bool      `json:"isAdmin"`
	BridgeChangesLocked bool      `json:"isUserBridgeChangesLocked"`
	Free                bool      `json:"isFree"`
	DeletedAt           time.Time `json:"deletedAt"`
	SupportRoomID       id.RoomID `json:"supportRoomId"`
	UseHungryserv       bool      `json:"useHungryserv"`
	BridgeClusterID     string    `json:"bridgeClusterId"`
}

type RespWhoami struct {
	User     WhoamiUser     `json:"user"`
	UserInfo WhoamiUserInfo `json:"userInfo"`
}

var cli = &http.Client{Timeout: 30 * time.Second}

func newRequest(baseDomain, token, method, path string) *http.Request {
	req := &http.Request{
		URL: &url.URL{
			Scheme: "https",
			Host:   fmt.Sprintf("api.%s", baseDomain),
			Path:   path,
		},
		Method: method,
		Header: http.Header{
			"Authorization": {fmt.Sprintf("Bearer %s", token)},
			"User-Agent":    {mautrix.DefaultUserAgent},
		},
	}
	if method == http.MethodPut || method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

type ReqPostBridgeState struct {
	StateEvent status.BridgeStateEvent `json:"stateEvent"`
	Reason     string                  `json:"reason"`
	Info       map[string]any          `json:"info"`
}

func DeleteBridge(domain, bridgeName, token string) error {
	req := newRequest(domain, token, http.MethodDelete, fmt.Sprintf("/bridge/%s", bridgeName))
	r, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer r.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		if body != nil {
			errorMsg, ok := body["error"].(string)
			if ok {
				return fmt.Errorf("server returned error (HTTP %d): %s", r.StatusCode, errorMsg)
			}
		}
		return fmt.Errorf("unexpected status code %d", r.StatusCode)
	}
	return nil
}

func PostBridgeState(domain, username, bridgeName, asToken string, data ReqPostBridgeState) error {
	req := newRequest(domain, asToken, http.MethodPost, fmt.Sprintf("/bridgebox/%s/bridge/%s/bridge_state", username, bridgeName))
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(&data)
	if err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}
	req.Body = io.NopCloser(&buf)
	r, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer r.Body.Close()
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code %d", r.StatusCode)
	}
	return nil
}

func Whoami(baseDomain, token string) (resp *RespWhoami, err error) {
	r, err := cli.Do(newRequest(baseDomain, token, http.MethodGet, "/whoami"))
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d", r.StatusCode)
	}
	err = json.NewDecoder(r.Body).Decode(&resp)
	if err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}
	return resp, nil
}
