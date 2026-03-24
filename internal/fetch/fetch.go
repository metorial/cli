package fetch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/version"
	metorial "github.com/metorial/metorial-go/v1"
)

const userAgent = "metorial-cli"

type Options struct {
	Target   string
	Method   string
	Headers  []string
	Data     string
	BodyFile string
	Include  bool
}

type Response struct {
	StatusCode int
	Status     string
	Headers    map[string][]string
	Body       []byte
}

func Execute(runtime config.Runtime, opts Options, stdin io.Reader) (*Response, error) {
	return execute(runtime, opts, stdin, true)
}

func execute(runtime config.Runtime, opts Options, stdin io.Reader, allowRefresh bool) (*Response, error) {
	requestURL, err := ResolveURL(runtime.APIHostURL, opts.Target)
	if err != nil {
		return nil, err
	}

	body, err := ResolveBody(opts, stdin)
	if err != nil {
		return nil, err
	}

	headers, err := ParseHeaders(opts.Headers)
	if err != nil {
		return nil, err
	}

	payload := []byte(nil)
	if body != nil {
		payload, err = io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("metorial: failed to read request body: %w", err)
		}
	}

	method := resolveMethod(opts.Method, len(payload) > 0)

	headerMap := map[string]string{
		"User-Agent": userAgent + "/" + version.Version,
	}
	for key, values := range headers {
		headerMap[key] = strings.Join(values, ", ")
	}

	if len(payload) > 0 && headerMap["Content-Type"] == "" && looksLikeJSONBody(payload) {
		headerMap["Content-Type"] = "application/json"
	}

	sdk, err := runtime.SDK()
	if err != nil {
		return nil, err
	}

	response, err := sdk.Fetch(&metorial.RawRequest{
		Method:  method,
		URL:     requestURL.String(),
		Headers: headerMap,
		Body:    payload,
	})
	if response == nil {
		return nil, err
	}

	result := &Response{
		StatusCode: response.StatusCode,
		Status:     response.Status,
		Headers:    response.Headers,
		Body:       response.Body,
	}

	if result.StatusCode == 401 && allowRefresh && runtime.Refresh != nil {
		refreshedRuntime, refreshErr := runtime.Refresh(true)
		if refreshErr == nil && strings.TrimSpace(refreshedRuntime.APIKey) != "" && refreshedRuntime.APIKey != runtime.APIKey {
			return execute(refreshedRuntime, opts, bytes.NewReader(payload), false)
		}
	}

	return result, err
}

func ResolveURL(apiHostURL *url.URL, target string) (*url.URL, error) {
	value := strings.TrimSpace(target)
	if value == "" {
		return nil, fmt.Errorf("metorial: request path or URL is required")
	}

	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil {
			return nil, fmt.Errorf("metorial: invalid request URL %q: %w", target, err)
		}

		if !sameHost(apiHostURL, parsed) {
			return nil, fmt.Errorf("metorial: request URL host %q does not match selected API host %q", parsed.Host, apiHostURL.Host)
		}

		return parsed, nil
	}

	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("metorial: invalid request path %q: %w", target, err)
	}

	return apiHostURL.ResolveReference(parsed), nil
}

func ParseHeaders(input []string) (map[string][]string, error) {
	headers := map[string][]string{}

	for _, raw := range input {
		key, value, ok := strings.Cut(raw, ":")
		if !ok {
			return nil, fmt.Errorf("metorial: invalid header %q, expected KEY: VALUE", raw)
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return nil, fmt.Errorf("metorial: invalid header %q, header name cannot be empty", raw)
		}

		headers[key] = append(headers[key], value)
	}

	return headers, nil
}

func ResolveBody(opts Options, stdin io.Reader) (io.Reader, error) {
	if opts.Data != "" && opts.BodyFile != "" {
		return nil, fmt.Errorf("metorial: use either --data or --body-file, not both")
	}

	switch {
	case opts.Data != "":
		if opts.Data == "@-" {
			content, err := io.ReadAll(stdin)
			if err != nil {
				return nil, fmt.Errorf("metorial: failed to read request body from stdin: %w", err)
			}
			return bytes.NewReader(content), nil
		}
		return strings.NewReader(opts.Data), nil
	case opts.BodyFile != "":
		if opts.BodyFile == "-" {
			content, err := io.ReadAll(stdin)
			if err != nil {
				return nil, fmt.Errorf("metorial: failed to read request body from stdin: %w", err)
			}
			return bytes.NewReader(content), nil
		}

		content, err := os.ReadFile(opts.BodyFile)
		if err != nil {
			return nil, fmt.Errorf("metorial: failed to read body file %q: %w", opts.BodyFile, err)
		}
		return bytes.NewReader(content), nil
	default:
		return nil, nil
	}
}

func resolveMethod(input string, hasBody bool) string {
	if strings.TrimSpace(input) != "" {
		return strings.ToUpper(strings.TrimSpace(input))
	}
	if hasBody {
		return "POST"
	}
	return "GET"
}

func sameHost(left, right *url.URL) bool {
	return strings.EqualFold(left.Host, right.Host)
}

func looksLikeJSONBody(payload []byte) bool {
	var value any
	return json.Unmarshal(bytes.TrimSpace(payload), &value) == nil
}
