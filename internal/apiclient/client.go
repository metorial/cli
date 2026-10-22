package apiclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/fetch"
	metorial "github.com/metorial/metorial-go/v1"
)

type Pagination struct {
	HasMoreBefore bool   `json:"has_more_before"`
	HasMoreAfter  bool   `json:"has_more_after"`
	After         string `json:"after,omitempty"`
}

type ListOptions struct {
	Limit  float64
	After  string
	Before string
	Cursor string
}

type Client struct {
	Runtime  config.Runtime
	Headers  map[string]string
	Describe func(target string, err error) error
}

func (c Client) Do(method string, target string, body any, output any) error {
	var payload []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("metorial: failed to encode request body: %w", err)
		}
		payload = encoded
	}

	headers := map[string]string{}
	if payload != nil {
		headers["Content-Type"] = "application/json"
	}

	return c.send(fetch.Request{Method: method, Target: target, Headers: headers, Body: payload}, output)
}

func (c Client) send(request fetch.Request, output any) error {
	headers := map[string]string{}
	for key, value := range c.Headers {
		headers[key] = value
	}
	for key, value := range request.Headers {
		headers[key] = value
	}
	request.Headers = headers

	response, err := fetch.Do(c.Runtime, request)
	if err != nil {
		return c.describe(request.Target, err)
	}

	if output == nil || len(response.Body) == 0 {
		return nil
	}
	if err := json.Unmarshal(response.Body, output); err != nil {
		return fmt.Errorf("metorial: failed to decode response from %s: %w", request.Target, err)
	}

	return nil
}

func (c Client) describe(target string, err error) error {
	if c.Describe == nil {
		return err
	}
	return c.Describe(target, err)
}

func IsStatus(err error, statusCode int) bool {
	apiErr := &metorial.Error{}
	return errors.As(err, &apiErr) && apiErr.StatusCode == statusCode
}

func ListQuery(options ListOptions, extra url.Values) string {
	values := url.Values{}
	for key, items := range extra {
		for _, item := range items {
			values.Add(key, item)
		}
	}

	if options.Limit > 0 {
		values.Set("limit", strconv.FormatFloat(options.Limit, 'f', -1, 64))
	}
	if after := strings.TrimSpace(options.After); after != "" {
		values.Set("after", after)
	}
	if before := strings.TrimSpace(options.Before); before != "" {
		values.Set("before", before)
	}
	if cursor := strings.TrimSpace(options.Cursor); cursor != "" {
		values.Set("cursor", cursor)
	}

	if len(values) == 0 {
		return ""
	}
	return "?" + values.Encode()
}

func PathSegment(value string) string {
	return url.PathEscape(strings.TrimSpace(value))
}
