package skillsapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/metorial/cli/internal/apiclient"
	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/fileupload"
	metorial "github.com/metorial/metorial-go/v1"
)

const (
	maxUploadBytes   = 100 * 1024 * 1024
	maxDownloadBytes = 200 * 1024 * 1024
	uploadPurpose    = "generic"

	// Pinned: the consumer API version has no skill endpoints.
	APIVersion = "mt_2026_01_01_magnetar"
)

var downloadClient = fileupload.NewTransferClient()

type Client struct {
	api apiclient.Client
}

func New(runtime config.Runtime) (*Client, error) {
	if err := runtime.RequireAPIKey(); err != nil {
		return nil, err
	}

	return &Client{
		api: apiclient.Client{
			Runtime:  runtime,
			Headers:  map[string]string{"Metorial-Version": APIVersion},
			Describe: describeError,
		},
	}, nil
}

func (c *Client) APIHost() string {
	return c.api.Runtime.APIHost
}

func (c *Client) ListSkills(options ListOptions, search string) (*SkillList, error) {
	extra := url.Values{}
	if search = strings.TrimSpace(search); search != "" {
		extra.Set("search", search)
	}

	result := &SkillList{}
	return result, c.api.Do(http.MethodGet, "/skills"+apiclient.ListQuery(options, extra), nil, result)
}

func (c *Client) GetSkill(skillID string) (*Skill, error) {
	result := &Skill{}
	return result, c.api.Do(http.MethodGet, "/skills/"+apiclient.PathSegment(skillID), nil, result)
}

// Older backends resolve only IDs and can't search slugs, hence the paging fallback.
func (c *Client) ResolveSkill(reference string) (*Skill, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return nil, errors.New("metorial: a skill ID or slug is required")
	}

	skill, err := c.GetSkill(reference)
	if err == nil {
		return skill, nil
	}
	if !apiclient.IsStatus(err, http.StatusNotFound) {
		return nil, err
	}

	options := ListOptions{Limit: 100}
	for {
		page, err := c.ListSkills(options, "")
		if err != nil {
			return nil, err
		}
		for _, candidate := range page.Items {
			if candidate.Slug != nil && *candidate.Slug == reference {
				return &candidate, nil
			}
		}

		if !page.Pagination.HasMoreAfter || len(page.Items) == 0 {
			break
		}
		after := page.Pagination.After
		if after == "" {
			after = page.Items[len(page.Items)-1].ID
		}
		if after == options.After {
			break
		}
		options.After = after
	}

	return nil, fmt.Errorf("metorial: no skill found with ID or slug %q", reference)
}

func (c *Client) CreateSkill(input CreateSkillInput) (*Skill, error) {
	result := &Skill{}
	return result, c.api.Do(http.MethodPost, "/skills", input, result)
}

func (c *Client) ListFiles(skillID string) (*FileList, error) {
	result := &FileList{}
	return result, c.api.Do(http.MethodGet, filesTarget(skillID, ""), nil, result)
}

func (c *Client) GetFile(skillID string, filePath string) (*FileContent, error) {
	query := url.Values{}
	query.Set("path", filePath)

	result := &FileContent{}
	return result, c.api.Do(http.MethodGet, filesTarget(skillID, "/content")+"?"+query.Encode(), nil, result)
}

func (c *Client) PutFile(skillID string, change ChangeInput) (*FileChange, error) {
	result := &FileChange{}
	return result, c.api.Do(http.MethodPut, filesTarget(skillID, ""), change, result)
}

func (c *Client) DeleteFile(skillID string, filePath string, ifMatch string) (*FileChange, error) {
	query := url.Values{}
	query.Set("path", filePath)
	if ifMatch != "" {
		query.Set("if_match", ifMatch)
	}

	result := &FileChange{}
	return result, c.api.Do(http.MethodDelete, filesTarget(skillID, "")+"?"+query.Encode(), nil, result)
}

func (c *Client) MoveFile(skillID string, change ChangeInput) (*FileChange, error) {
	result := &FileChange{}
	return result, c.api.Do(http.MethodPost, filesTarget(skillID, "/move"), change, result)
}

func (c *Client) ApplyChanges(skillID string, changes []ChangeInput) (*FileChangeList, error) {
	body := map[string]any{"changes": changes}

	result := &FileChangeList{}
	return result, c.api.Do(http.MethodPost, filesTarget(skillID, "/batch"), body, result)
}

func (c *Client) ReadFile(skillID string, filePath string) (*FileContent, []byte, error) {
	file, err := c.GetFile(skillID, filePath)
	if err != nil {
		return nil, nil, err
	}

	if file.Content != nil {
		return file, []byte(*file.Content), nil
	}
	if file.DownloadURL == nil || strings.TrimSpace(*file.DownloadURL) == "" {
		return nil, nil, fmt.Errorf("metorial: the content of %s cannot be downloaded from this deployment", file.Path)
	}

	content, err := download(*file.DownloadURL)
	if err != nil {
		return nil, nil, fmt.Errorf("metorial: failed to download %s: %w", file.Path, err)
	}

	return file, content, nil
}

func (c *Client) UploadFile(name string, content []byte) (*UploadedFile, error) {
	if len(content) > maxUploadBytes {
		return nil, fmt.Errorf("metorial: %s is larger than %d MB", name, maxUploadBytes/1024/1024)
	}

	uploader := fileupload.New(c.api.Runtime)
	uploader.Headers = c.api.Headers

	response, err := uploader.Upload(context.Background(), fileupload.BytesSource(name, content), fileupload.Options{
		Purpose:    uploadPurpose,
		InstanceID: c.api.Runtime.InstanceID,
	})
	if err != nil {
		return nil, describeError("/files", err)
	}

	result := &UploadedFile{}
	if err := json.Unmarshal(response.Body, result); err != nil {
		return nil, fmt.Errorf("metorial: failed to decode response from /files: %w", err)
	}
	return result, nil
}

func download(target string) ([]byte, error) {
	response, err := downloadClient.Get(target)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("download returned %s", response.Status)
	}

	content, err := io.ReadAll(io.LimitReader(response.Body, maxDownloadBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxDownloadBytes {
		return nil, fmt.Errorf("file is larger than %d MB", maxDownloadBytes/1024/1024)
	}

	return content, nil
}

func describeError(_ string, err error) error {
	apiErr := &metorial.Error{}
	if !errors.As(err, &apiErr) {
		return err
	}

	switch apiErr.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("metorial: your session is no longer valid: %s\nSign in again with the login command", apiErr.Message)
	case http.StatusForbidden:
		return fmt.Errorf("metorial: you do not have access to this skill: %s", apiErr.Message)
	}

	return err
}

func IsConflict(err error) bool {
	return apiclient.IsStatus(err, http.StatusPreconditionFailed)
}

func filesTarget(skillID string, suffix string) string {
	return "/skills/" + apiclient.PathSegment(skillID) + "/files" + suffix
}
