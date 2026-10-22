package skillsapi

import (
	"encoding/json"
	"time"

	"github.com/metorial/cli/internal/apiclient"
)

const (
	FileKindDocument = "document"
	FileKindFile     = "file"

	ChangeTypePut    = "put"
	ChangeTypeDelete = "delete"
	ChangeTypeMove   = "move"
)

type ListOptions = apiclient.ListOptions

type Skill struct {
	Object      string    `json:"object"`
	ID          string    `json:"id"`
	Status      string    `json:"status"`
	Slug        *string   `json:"slug"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	StoreID     string    `json:"store_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type SkillList struct {
	Items      []Skill              `json:"items"`
	Pagination apiclient.Pagination `json:"pagination"`
}

type CreateSkillInput struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	TemplateID  string `json:"template_id,omitempty"`
}

type File struct {
	Object      string    `json:"object"`
	Path        string    `json:"path"`
	Kind        string    `json:"kind"`
	Size        int64     `json:"size"`
	Etag        string    `json:"etag"`
	SkillID     string    `json:"skill_id"`
	StoreItemID string    `json:"store_item_id"`
	DocumentID  *string   `json:"document_id"`
	FileID      *string   `json:"file_id"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type FileList struct {
	Items []File `json:"items"`
}

type FileContent struct {
	File
	Content     *string `json:"content"`
	DownloadURL *string `json:"download_url"`
}

type FileChange struct {
	Object       string  `json:"object"`
	Type         string  `json:"type"`
	Path         string  `json:"path"`
	PreviousPath *string `json:"previous_path"`
	File         *File   `json:"file"`
}

type FileChangeList struct {
	Items []FileChange `json:"items"`
}

// IfMatch: omitted skips the check, null requires an empty path, a string is the required etag.
type ChangeInput struct {
	Type    string          `json:"type,omitempty"`
	Path    string          `json:"path"`
	ToPath  string          `json:"to_path,omitempty"`
	Content *string         `json:"content,omitempty"`
	FileID  string          `json:"file_id,omitempty"`
	IfMatch json.RawMessage `json:"if_match,omitempty"`
}

type UploadedFile struct {
	Object string `json:"object"`
	ID     string `json:"id"`
}
