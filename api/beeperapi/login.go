package beeperapi

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"
)

type RespStartLogin struct {
	RequestID string    `json:"request"`
	Type      []string  `json:"type"`
	Expires   time.Time `json:"expires"`
}

type ReqSendLoginEmail struct {
	RequestID            string `json:"request"`
	Email                string `json:"email"`
	AppType              string `json:"appType"`
	OnlyExistingAccounts bool   `json:"onlyExistingAccounts"`
}

type ReqSendLoginCode struct {
	RequestID            string `json:"request"`
	Code                 string `json:"response"`
	AppType              string `json:"appType"`
	OnlyExistingAccounts bool   `json:"onlyExistingAccounts"`
}

type RespSendLoginCode struct {
	LoginToken string      `json:"token"`
	Whoami     *RespWhoami `json:"whoami"`
}

var ErrInvalidLoginCode = fmt.Errorf("invalid login code")

const loginAuth = "BEEPER-PRIVATE-API-PLEASE-DONT-USE"

func StartLogin(baseDomain string) (resp *RespStartLogin, err error) {
	req := newRequest(baseDomain, loginAuth, http.MethodPost, "/user/login")
	req.Body = io.NopCloser(bytes.NewReader([]byte("{}")))
	err = doRequest(req, nil, &resp)
	return
}

func SendLoginEmail(baseDomain, request, email string) error {
	req := newRequest(baseDomain, loginAuth, http.MethodPost, "/user/login/email")
	reqData := &ReqSendLoginEmail{
		RequestID:            request,
		Email:                email,
		AppType:              "bbctl",
		OnlyExistingAccounts: true,
	}
	return doRequest(req, reqData, nil)
}

func SendLoginCode(baseDomain, request, code string) (resp *RespSendLoginCode, err error) {
	req := newRequest(baseDomain, loginAuth, http.MethodPost, "/user/login/response")
	reqData := &ReqSendLoginCode{
		RequestID:            request,
		Code:                 code,
		AppType:              "bbctl",
		OnlyExistingAccounts: true,
	}
	err = doRequest(req, reqData, &resp)
	return
}
