package gitlab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"maunium.net/go/mautrix"
)

var cli = &http.Client{Timeout: 30 * time.Second}

type queryRequestBody struct {
	Query     string `json:"query"`
	Variables any    `json:"variables"`
}

type QueryErrorLocation struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

type QueryErrorItem struct {
	Message   string               `json:"message"`
	Locations []QueryErrorLocation `json:"locations"`
}

type QueryError []QueryErrorItem

func (qe QueryError) Error() string {
	if len(qe) == 1 {
		return qe[0].Message
	}
	plural := "s"
	if len(qe) == 2 {
		plural = ""
	}
	return fmt.Sprintf("%s (and %d other error%s)", qe[0].Message, len(qe)-1, plural)
}

type queryResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors QueryError      `json:"errors"`
}

func graphqlQuery(domain, query string, args any) (json.RawMessage, error) {
	req := &http.Request{
		URL: &url.URL{
			Scheme: "https",
			Host:   domain,
			Path:   "/api/graphql",
		},
		Method: http.MethodPost,
		Header: http.Header{
			"User-Agent":   {mautrix.DefaultUserAgent},
			"Content-Type": {"application/json"},
			"Accept":       {"application/json"},
		},
	}
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(queryRequestBody{
		Query:     query,
		Variables: args,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode request body: %w", err)
	}
	req.Body = io.NopCloser(&buf)
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	var respData queryResponse
	err = json.NewDecoder(resp.Body).Decode(&respData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response body: %w", err)
	}
	if len(respData.Errors) > 0 {
		return nil, respData.Errors
	}
	return respData.Data, nil
}
