package fileupload

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/fetch"
)

const (
	filesEndpoint            = "/files"
	signedUploadsUnavailable = "Signed upload URLs are not available"
)

type Options struct {
	Purpose      string
	Title        string
	StoreID      string
	StorePath    string
	StoreReplace bool
	InstanceID   string
}

type Source struct {
	Name        string
	ContentType string
	Size        int64
	Open        func() (io.ReadCloser, error)
}

func FileSource(filePath, name, contentType string, size int64) Source {
	return Source{
		Name:        name,
		ContentType: contentType,
		Size:        size,
		Open:        func() (io.ReadCloser, error) { return os.Open(filePath) },
	}
}

func BytesSource(name string, content []byte) Source {
	return Source{
		Name:        name,
		ContentType: ContentTypeFor(name),
		Size:        int64(len(content)),
		Open:        func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(content)), nil },
	}
}

func ContentTypeFor(name string) string {
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		return contentType
	}
	return "application/octet-stream"
}

type Uploader struct {
	Runtime config.Runtime
	// Never sent to the storage upload.
	Headers map[string]string
	Client  *http.Client
}

func New(runtime config.Runtime) *Uploader {
	return &Uploader{Runtime: runtime, Client: NewTransferClient()}
}

func NewTransferClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = time.Minute
	return &http.Client{Transport: transport, Timeout: 30 * time.Minute}
}

// Returns the API response even on error so callers can render the error body.
func (u *Uploader) Upload(ctx context.Context, source Source, options Options) (*fetch.Response, error) {
	request := map[string]any{
		"mode":      "get_upload_url",
		"purpose":   options.Purpose,
		"file_name": source.Name,
		"file_size": source.Size,
		"file_type": source.ContentType,
	}
	if options.Title != "" {
		request["title"] = options.Title
	}
	if options.InstanceID != "" {
		request["instance_id"] = options.InstanceID
	}
	if options.StoreID != "" {
		request["store_id"] = options.StoreID
		request["path"] = options.StorePath
		request["store_replace"] = options.StoreReplace
	}

	response, err := u.postJSON(request)
	if err != nil {
		if response != nil && response.StatusCode == http.StatusBadRequest && strings.Contains(err.Error(), signedUploadsUnavailable) {
			err = fmt.Errorf("%w\nThe server's file storage can't issue signed upload URLs; configure UPLOAD_HOST or presign-capable object storage", err)
		}
		return response, err
	}

	var session struct {
		ID     string `json:"id"`
		Upload struct {
			URL    string `json:"url"`
			Method string `json:"method"`
		} `json:"upload"`
	}
	if err := json.Unmarshal(response.Body, &session); err != nil {
		return nil, fmt.Errorf("metorial: invalid upload session response: %w", err)
	}
	if session.ID == "" || session.Upload.URL == "" {
		return nil, errors.New("metorial: the API did not return an upload URL")
	}

	if err := u.put(ctx, session.Upload.Method, session.Upload.URL, source); err != nil {
		return nil, err
	}

	complete := map[string]any{
		"mode":           "complete",
		"file_upload_id": session.ID,
	}
	if options.InstanceID != "" {
		complete["instance_id"] = options.InstanceID
	}
	return u.postJSON(complete)
}

func (u *Uploader) postJSON(body map[string]any) (*fetch.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{}
	for key, value := range u.Headers {
		headers[key] = value
	}
	headers["Content-Type"] = "application/json"

	return fetch.Do(u.Runtime, fetch.Request{
		Method:  http.MethodPost,
		Target:  filesEndpoint,
		Headers: headers,
		Body:    payload,
	})
}

// put sends the file to the presigned storage URL. The URL signature covers the
// content type, so it must match the file_type declared when requesting the URL,
// and Metorial credentials must not be forwarded to the storage backend.
func (u *Uploader) put(ctx context.Context, method, url string, source Source) error {
	if method == "" {
		method = http.MethodPut
	}

	content, err := source.Open()
	if err != nil {
		return fmt.Errorf("metorial: %w", err)
	}
	defer content.Close()

	request, err := http.NewRequestWithContext(ctx, method, url, content)
	if err != nil {
		return fmt.Errorf("metorial: invalid upload URL: %w", err)
	}
	request.ContentLength = source.Size
	request.Header.Set("Content-Type", source.ContentType)

	response, err := u.Client.Do(request)
	if err != nil {
		return fmt.Errorf("metorial: failed to upload %s: %w", source.Name, err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode > 299 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		message := strings.TrimSpace(string(detail))
		if message == "" {
			return fmt.Errorf("metorial: failed to upload %s: storage responded with %s", source.Name, response.Status)
		}
		return fmt.Errorf("metorial: failed to upload %s: storage responded with %s: %s", source.Name, response.Status, message)
	}

	_, _ = io.Copy(io.Discard, response.Body)
	return nil
}
