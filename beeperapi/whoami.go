package beeperapi

import (
	"time"

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

func Whoami(baseDomain, accessToken string) (resp *RespWhoami, err error) {
	return nil, nil
}
