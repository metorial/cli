package files

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/fetch"
)

// Must stay in sync with maxDirectUploadSize in the files API.
const maxDirectUploadSize = 100 * 1024 * 1024

const filesEndpoint = "/files"

type uploadOptions struct {
	Purpose      string
	Title        string
	StoreID      string
	StorePath    string
	StoreReplace bool
	InstanceID   string
}

type uploadSource struct {
	Path        string
	Name        string
	ContentType string
	Size        int64
}

type uploader struct {
	runtime     config.Runtime
	client      *http.Client
	directLimit int64
}

func newUploader(runtime config.Runtime) *uploader {
	return &uploader{
		runtime:     runtime,
		client:      http.DefaultClient,
		directLimit: maxDirectUploadSize,
	}
}

// upload returns the API response alongside any error so callers can render
// API error bodies the same way as other commands.
func (u *uploader) upload(ctx context.Context, source uploadSource, options uploadOptions) (*fetch.Response, error) {
	if source.Size <= u.directLimit {
		return u.uploadDirect(source, options)
	}
	return u.uploadWithURL(ctx, source, options)
}

func (u *uploader) uploadDirect(source uploadSource, options uploadOptions) (*fetch.Response, error) {
	body, contentType, err := multipartBody(source, options)
	if err != nil {
		return nil, err
	}

	return fetch.Do(u.runtime, fetch.Request{
		Method:  http.MethodPost,
		Target:  filesEndpoint,
		Headers: map[string]string{"Content-Type": contentType},
		Body:    body,
	})
}

func (u *uploader) uploadWithURL(ctx context.Context, source uploadSource, options uploadOptions) (*fetch.Response, error) {
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

func (u *uploader) postJSON(body map[string]any) (*fetch.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	return fetch.Do(u.runtime, fetch.Request{
		Method:  http.MethodPost,
		Target:  filesEndpoint,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    payload,
	})
}

// put sends the file to the presigned storage URL. The URL signature covers the
// content type, so it must match the file_type declared when requesting the URL,
// and Metorial credentials must not be forwarded to the storage backend.
func (u *uploader) put(ctx context.Context, method, url string, source uploadSource) error {
	if method == "" {
		method = http.MethodPut
	}

	file, err := os.Open(source.Path)
	if err != nil {
		return fmt.Errorf("metorial: %w", err)
	}
	defer file.Close()

	request, err := http.NewRequestWithContext(ctx, method, url, file)
	if err != nil {
		return fmt.Errorf("metorial: invalid upload URL: %w", err)
	}
	request.ContentLength = source.Size
	request.Header.Set("Content-Type", source.ContentType)

	response, err := u.client.Do(request)
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

func multipartBody(source uploadSource, options uploadOptions) ([]byte, string, error) {
	buffer := &bytes.Buffer{}
	writer := multipart.NewWriter(buffer)

	fields := [][2]string{{"purpose", options.Purpose}}
	if options.Title != "" {
		fields = append(fields, [2]string{"title", options.Title})
	}
	if options.InstanceID != "" {
		fields = append(fields, [2]string{"instance_id", options.InstanceID})
	}
	if options.StoreID != "" {
		fields = append(fields,
			[2]string{"store_id", options.StoreID},
			[2]string{"path", options.StorePath},
			[2]string{"store_replace", strconv.FormatBool(options.StoreReplace)},
		)
	}
	for _, field := range fields {
		if err := writer.WriteField(field[0], field[1]); err != nil {
			return nil, "", err
		}
	}

	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, quoteEscaper.Replace(source.Name)))
	header.Set("Content-Type", source.ContentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return nil, "", err
	}
	if err := copyFile(part, source.Path); err != nil {
		return nil, "", fmt.Errorf("metorial: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}

var quoteEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`)

func contentTypeFor(name string) string {
	if contentType := mime.TypeByExtension(filepath.Ext(name)); contentType != "" {
		return contentType
	}
	return "application/octet-stream"
}
