package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/urfave/cli/v2"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/appservice"
	"maunium.net/go/mautrix/bridge/status"
)

var proxyCommand = &cli.Command{
	Name:    "proxy",
	Aliases: []string{"x"},
	Usage:   "Connect to an appservice websocket, and proxy it to a local appservice HTTP server",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "registration",
			Required: true,
			Aliases:  []string{"r"},
			EnvVars:  []string{"BEEPER_BRIDGE_REGISTRATION_FILE"},
			Usage:    "The path to the registration file to read the as_token, hs_token and local appservice URL from",
		},
	},
	Action: proxyAppserviceWebsocket,
}

const defaultReconnectBackoff = 2 * time.Second
const maxReconnectBackoff = 2 * time.Minute
const reconnectBackoffReset = 5 * time.Minute

func runAppserviceWebsocket(ctx context.Context, doneCallback func(), as *appservice.AppService) {
	defer doneCallback()
	reconnectBackoff := defaultReconnectBackoff
	lastDisconnect := time.Now()
	for {
		err := as.StartWebsocket("", func() {
			// TODO support states properly instead of just sending unconfigured
			_ = as.SendWebsocket(&appservice.WebsocketRequest{
				Command: "bridge_status",
				Data:    &status.BridgeState{StateEvent: status.StateUnconfigured},
			})
		})
		if errors.Is(err, appservice.ErrWebsocketManualStop) {
			return
		} else if closeCommand := (&appservice.CloseCommand{}); errors.As(err, &closeCommand) && closeCommand.Status == appservice.MeowConnectionReplaced {
			as.Log.Info().Msg("Appservice websocket closed by another connection, shutting down...")
			return
		} else if err != nil {
			as.Log.Err(err).Msg("Error in appservice websocket")
		}
		if ctx.Err() != nil {
			return
		}
		now := time.Now()
		if lastDisconnect.Add(reconnectBackoffReset).Before(now) {
			reconnectBackoff = defaultReconnectBackoff
		} else {
			reconnectBackoff *= 2
			if reconnectBackoff > maxReconnectBackoff {
				reconnectBackoff = maxReconnectBackoff
			}
		}
		lastDisconnect = now
		as.Log.Info().
			Int("backoff_seconds", int(reconnectBackoff.Seconds())).
			Msg("Websocket disconnected, reconnecting after a while...")
		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnectBackoff):
		}
	}
}

var wsProxyClient = http.Client{Timeout: 10 * time.Second}

func proxyWebsocketTransaction(ctx context.Context, hsToken string, baseURL *url.URL, msg appservice.WebsocketMessage) error {
	log := zerolog.Ctx(ctx)
	log.Info().Object("contents", &msg.Transaction).Msg("Forwarding transaction")
	fullURL := mautrix.BuildURL(baseURL, "_matrix", "app", "v1", "transactions", msg.TxnID)
	var body bytes.Buffer
	err := json.NewEncoder(&body).Encode(&msg.Transaction)
	if err != nil {
		log.Err(err).Msg("Failed to re-encode transaction")
		return fmt.Errorf("failed to encode transaction: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, fullURL.String(), &body)
	if err != nil {
		log.Err(err).Msg("Failed to prepare transaction request")
		return fmt.Errorf("failed to prepare request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", hsToken))
	resp, err := wsProxyClient.Do(req)
	if err != nil {
		log.Err(err).Msg("Failed to send transaction request")
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	var errorResp mautrix.RespError
	if resp.StatusCode >= 300 {
		err = json.NewDecoder(resp.Body).Decode(&errorResp)
		if err != nil {
			log.Error().
				AnErr("json_decode_err", err).
				Int("status_code", resp.StatusCode).
				Msg("Got non-JSON error response sending transaction")
			return fmt.Errorf("http %d with non-JSON body", resp.StatusCode)
		}
		log.Err(errorResp).
			Int("status_code", resp.StatusCode).
			Msg("Got error response sending transaction")
		return fmt.Errorf("http %d: %s: %s", resp.StatusCode, errorResp.Err, errorResp.ErrCode)
	}
	return nil
}

func proxyWebsocketRequest(baseURL *url.URL, cmd appservice.WebsocketCommand) (bool, any) {
	var reqData appservice.HTTPProxyRequest
	if err := json.Unmarshal(cmd.Data, &reqData); err != nil {
		return false, fmt.Errorf("failed to parse proxy request: %w", err)
	}
	fullURL := baseURL.JoinPath(reqData.Path)
	fullURL.RawQuery = reqData.Query
	body := bytes.NewReader(reqData.Body)
	httpReq, err := http.NewRequestWithContext(cmd.Ctx, http.MethodPut, fullURL.String(), body)
	if err != nil {
		return false, fmt.Errorf("failed to prepare request: %w", err)
	}
	httpReq.Header = reqData.Headers
	resp, err := wsProxyClient.Do(httpReq)
	if err != nil {
		return false, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read request body: %w", err)
	}
	if !json.Valid(respData) {
		encodedData := make([]byte, 2+base64.RawStdEncoding.EncodedLen(len(respData)))
		encodedData[0] = '"'
		base64.RawStdEncoding.Encode(encodedData[1:], respData)
		encodedData[len(encodedData)-1] = '"'
		respData = encodedData
	}
	return true, &appservice.HTTPProxyResponse{
		Status:  resp.StatusCode,
		Headers: resp.Header,
		Body:    respData,
	}
}

func prepareAppserviceWebsocketProxy(ctx *cli.Context, as *appservice.AppService) {
	parsedURL, _ := url.Parse(as.Registration.URL)
	zerolog.TimeFieldFormat = time.RFC3339Nano
	as.Log = zerolog.New(zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
		w.TimeFormat = time.StampMilli
	})).With().Timestamp().Logger()
	as.PrepareWebsocket()
	as.WebsocketTransactionHandler = func(ctx context.Context, msg appservice.WebsocketMessage) (bool, any) {
		err := proxyWebsocketTransaction(ctx, as.Registration.ServerToken, parsedURL, msg)
		if err != nil {
			return false, err
		}
		return true, &appservice.WebsocketTransactionResponse{TxnID: msg.TxnID}
	}
	as.SetWebsocketCommandHandler(appservice.WebsocketCommandHTTPProxy, func(cmd appservice.WebsocketCommand) (bool, any) {
		if cmd.Ctx == nil {
			cmd.Ctx = ctx.Context
		}
		return proxyWebsocketRequest(parsedURL, cmd)
	})
	_ = as.SetHomeserverURL(GetEnvConfig(ctx).HungryAddress)
}

type wsPingData struct {
	Timestamp int64 `json:"timestamp"`
}

func keepaliveAppserviceWebsocket(ctx context.Context, doneCallback func(), as *appservice.AppService) {
	log := as.Log.With().Str("component", "websocket pinger").Logger()
	defer doneCallback()
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
		if !as.HasWebsocket() {
			log.Debug().Msg("Not pinging: websocket not connected")
			continue
		}
		var resp wsPingData
		start := time.Now()
		err := as.RequestWebsocket(ctx, &appservice.WebsocketRequest{
			Command: "ping",
			Data:    &wsPingData{Timestamp: time.Now().UnixMilli()},
		}, &resp)
		if ctx.Err() != nil {
			return
		}
		duration := time.Since(start)
		if err != nil {
			log.Warn().Err(err).Dur("duration", duration).Msg("Websocket ping returned error")
			as.StopWebsocket(fmt.Errorf("websocket ping returned error in %s: %w", duration, err))
		} else {
			serverTs := time.UnixMilli(resp.Timestamp)
			log.Debug().
				Dur("duration", duration).
				Dur("req_duration", serverTs.Sub(start)).
				Dur("resp_duration", time.Since(serverTs)).
				Msg("Websocket ping returned success")
		}
	}
}

func proxyAppserviceWebsocket(ctx *cli.Context) error {
	regPath := ctx.String("registration")
	reg, err := appservice.LoadRegistration(regPath)
	if err != nil {
		return fmt.Errorf("failed to load registration: %w", err)
	} else if reg.URL == "" || reg.URL == "websocket" {
		return UserError{"You must change the `url` field in the registration file to point at the local appservice HTTP server (e.g. `http://localhost:8080`)"}
	} else if !strings.HasPrefix(reg.URL, "http://") && !strings.HasPrefix(reg.URL, "https://") {
		return UserError{"`url` field in registration must start with http:// or https://"}
	}
	as := appservice.Create()
	as.Registration = reg
	as.HomeserverDomain = "beeper.local"
	prepareAppserviceWebsocketProxy(ctx, as)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	wsCtx, cancel := context.WithCancel(ctx.Context)
	var wg sync.WaitGroup
	wg.Add(2)
	go runAppserviceWebsocket(wsCtx, wg.Done, as)
	go keepaliveAppserviceWebsocket(wsCtx, wg.Done, as)

	<-c

	fmt.Println()
	cancel()
	as.Log.Info().Msg("Interrupt received, stopping...")
	as.StopWebsocket(appservice.ErrWebsocketManualStop)
	wg.Wait()
	return nil
}
