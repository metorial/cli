package skillstest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/skillsapi"
)

const SkillID = "skill_test"
const SkillSlug = "release-notes"

type file struct {
	kind    string
	content []byte
	etag    string
}

type pendingUpload struct {
	contentType string
	content     []byte
	uploaded    bool
}

type Server struct {
	URL string

	mu         sync.Mutex
	files      map[string]*file
	uploads    map[string][]byte
	pending    map[string]*pendingUpload
	skills     []map[string]any
	nextID     int
	Batches    int
	Uploads    int
	Downloads  int
	SkillLists int

	// Uploads a real signed URL would reject.
	StorageErrors []string

	// GET /skills/:id ignores slugs, like older backends.
	LegacySkillLookup bool

	FailPath string

	FailReadPath string

	FailUploads bool

	IgnoreAfterCursor bool
}

func NewServer(t *testing.T) *Server {
	t.Helper()

	fake := &Server{files: map[string]*file{}, uploads: map[string][]byte{}, pending: map[string]*pendingUpload{}}
	fake.skills = []map[string]any{fake.skill()}
	server := httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(server.Close)

	fake.URL = server.URL
	return fake
}

func (s *Server) Client(t *testing.T) *skillsapi.Client {
	t.Helper()

	hostURL, err := url.Parse(s.URL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	client, err := skillsapi.New(config.Runtime{APIKey: "test-token", APIHost: s.URL, APIHostURL: hostURL})
	if err != nil {
		t.Fatalf("skillsapi.New() error = %v", err)
	}

	return client
}

// Prepends skills so finding the test skill takes several pages.
func (s *Server) AddSkills(count int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	others := []map[string]any{}
	for index := range count {
		id := fmt.Sprintf("skill_other_%d", index)
		others = append(others, map[string]any{"object": "skill", "id": id, "slug": "other-" + id, "name": "Other", "status": "active"})
	}
	s.skills = append(others, s.skills...)
}

func (s *Server) SetFile(filePath string, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.setFileLocked(filePath, []byte(content))
}

func (s *Server) DeleteFile(filePath string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.files, filePath)
}

func (s *Server) Content(filePath string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.files[filePath]
	if !ok {
		return "", false
	}
	return string(stored.content), true
}

func (s *Server) setFileLocked(filePath string, content []byte) *file {
	kind := skillsapi.FileKindFile
	etag := ""
	if ext := strings.ToLower(path.Ext(filePath)); ext == ".md" || ext == ".markdown" {
		kind = skillsapi.FileKindDocument
		// The real document store strips trailing newlines.
		content = []byte(strings.TrimRight(string(content), "\n"))
		sum := sha256.Sum256(content)
		etag = hex.EncodeToString(sum[:])
	} else {
		s.nextID++
		etag = fmt.Sprintf("fil_%d", s.nextID)
	}

	stored := &file{kind: kind, content: content, etag: etag}
	s.files[filePath] = stored
	return stored
}

func (s *Server) presentLocked(filePath string) map[string]any {
	stored := s.files[filePath]
	return map[string]any{
		"object":     "skill.file",
		"path":       filePath,
		"kind":       stored.kind,
		"size":       len(stored.content),
		"etag":       stored.etag,
		"skill_id":   SkillID,
		"updated_at": "2026-01-01T00:00:00Z",
	}
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// The real API exposes these endpoints only on the magnetar version.
	if (strings.HasPrefix(r.URL.Path, "/skills") || r.URL.Path == "/files") && r.Header.Get("Metorial-Version") != skillsapi.APIVersion {
		writeJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "The requested endpoint could not be found."})
		return
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/skills":
		s.SkillLists++
		s.listSkillsLocked(w, r.URL.Query())

	case r.Method == http.MethodGet && r.URL.Path == "/skills/"+SkillID:
		writeJSON(w, http.StatusOK, s.skill())

	case r.Method == http.MethodGet && r.URL.Path == "/skills/"+SkillSlug && !s.LegacySkillLookup:
		writeJSON(w, http.StatusOK, s.skill())

	case r.Method == http.MethodGet && r.URL.Path == "/skills/"+SkillID+"/files":
		paths := make([]string, 0, len(s.files))
		for filePath := range s.files {
			paths = append(paths, filePath)
		}
		sort.Strings(paths)

		items := []any{}
		for _, filePath := range paths {
			items = append(items, s.presentLocked(filePath))
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})

	case r.Method == http.MethodGet && r.URL.Path == "/skills/"+SkillID+"/files/content":
		filePath := normalize(r.URL.Query().Get("path"))
		if filePath == s.FailReadPath {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_server_error", "message": filePath + " could not be read"})
			return
		}
		stored, ok := s.files[filePath]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "File not found"})
			return
		}

		presented := s.presentLocked(filePath)
		if stored.kind == skillsapi.FileKindDocument {
			presented["content"] = string(stored.content)
		} else {
			presented["download_url"] = s.URL + "/download" + filePath
		}
		writeJSON(w, http.StatusOK, presented)

	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/download/"):
		stored, ok := s.files[strings.TrimPrefix(r.URL.Path, "/download")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		s.Downloads++
		_, _ = w.Write(stored.content)

	case r.Method == http.MethodPut && r.URL.Path == "/skills/"+SkillID+"/files":
		s.applySingleLocked(w, r, skillsapi.ChangeTypePut)

	case r.Method == http.MethodPost && r.URL.Path == "/skills/"+SkillID+"/files/move":
		s.applySingleLocked(w, r, skillsapi.ChangeTypeMove)

	case r.Method == http.MethodDelete && r.URL.Path == "/skills/"+SkillID+"/files":
		change := map[string]any{"type": skillsapi.ChangeTypeDelete, "path": r.URL.Query().Get("path")}
		if r.URL.Query().Has("if_match") {
			change["if_match"] = r.URL.Query().Get("if_match")
		}
		s.writeChangesLocked(w, []map[string]any{change}, true)

	case r.Method == http.MethodPost && r.URL.Path == "/skills/"+SkillID+"/files/batch":
		body := struct {
			Changes []map[string]any `json:"changes"`
		}{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.Batches++
		s.writeChangesLocked(w, body.Changes, false)

	case r.Method == http.MethodPost && r.URL.Path == "/files":
		s.handleSignedUploadLocked(w, r)

	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/storage/"):
		s.storeUploadLocked(w, r)

	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": r.Method + " " + r.URL.Path})
	}
}

func (s *Server) handleSignedUploadLocked(w http.ResponseWriter, r *http.Request) {
	body := map[string]any{}
	_ = json.NewDecoder(r.Body).Decode(&body)

	if s.FailUploads {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": "bad_request", "message": "Signed upload URLs are not available for the configured object storage"})
		return
	}

	switch body["mode"] {
	case "get_upload_url":
		s.nextID++
		id := fmt.Sprintf("fup_%d", s.nextID)
		contentType, _ := body["file_type"].(string)
		s.pending[id] = &pendingUpload{contentType: contentType}
		writeJSON(w, http.StatusOK, map[string]any{
			"object": "file_upload",
			"id":     id,
			"upload": map[string]any{"url": s.URL + "/storage/" + id, "method": http.MethodPut},
		})

	case "complete":
		id, _ := body["file_upload_id"].(string)
		pending, ok := s.pending[id]
		if !ok || !pending.uploaded {
			writeJSON(w, http.StatusBadRequest, map[string]any{"code": "bad_request", "message": "No content has been uploaded for file upload " + id})
			return
		}
		delete(s.pending, id)
		s.nextID++
		s.Uploads++
		fileID := fmt.Sprintf("upl_%d", s.nextID)
		s.uploads[fileID] = pending.content
		writeJSON(w, http.StatusOK, map[string]any{"object": "file", "id": fileID})

	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": "bad_request", "message": "mode must be one of get_upload_url, complete"})
	}
}

func (s *Server) storeUploadLocked(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/storage/")
	pending, ok := s.pending[id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Header.Get("Authorization") != "" {
		s.StorageErrors = append(s.StorageErrors, id+": sent Metorial credentials")
	}
	if got := r.Header.Get("Content-Type"); got != pending.contentType {
		s.StorageErrors = append(s.StorageErrors, fmt.Sprintf("%s: content type %q does not match %q", id, got, pending.contentType))
	}

	pending.content, _ = io.ReadAll(r.Body)
	pending.uploaded = true
	w.WriteHeader(http.StatusOK)
}

func (s *Server) applySingleLocked(w http.ResponseWriter, r *http.Request, changeType string) {
	change := map[string]any{}
	_ = json.NewDecoder(r.Body).Decode(&change)
	change["type"] = changeType
	s.writeChangesLocked(w, []map[string]any{change}, true)
}

func (s *Server) writeChangesLocked(w http.ResponseWriter, changes []map[string]any, single bool) {
	results, status, code, message := s.applyLocked(changes)
	switch {
	case status != http.StatusOK:
		writeJSON(w, status, map[string]any{"code": code, "message": message})
	case single:
		writeJSON(w, http.StatusOK, results[0])
	default:
		writeJSON(w, http.StatusOK, map[string]any{"items": results})
	}
}

// Like the real API: stops at the first failing change and keeps earlier ones.
func (s *Server) applyLocked(changes []map[string]any) ([]any, int, string, string) {
	results := []any{}
	for _, change := range changes {
		filePath := normalize(stringField(change, "path"))
		changeType := stringField(change, "type")

		if status, code, message := s.checkLocked(change, filePath, changeType); status != http.StatusOK {
			return nil, status, code, message
		}

		switch changeType {
		case skillsapi.ChangeTypeDelete:
			delete(s.files, filePath)
			results = append(results, map[string]any{"object": "skill.file_change", "type": changeType, "path": filePath, "previous_path": nil, "file": nil})

		case skillsapi.ChangeTypeMove:
			toPath := normalize(stringField(change, "to_path"))
			s.files[toPath] = s.files[filePath]
			delete(s.files, filePath)
			results = append(results, map[string]any{"object": "skill.file_change", "type": changeType, "path": toPath, "previous_path": filePath, "file": s.presentLocked(toPath)})

		default:
			content := []byte(stringField(change, "content"))
			if fileID := stringField(change, "file_id"); fileID != "" {
				content = s.uploads[fileID]
			}
			s.setFileLocked(filePath, content)
			results = append(results, map[string]any{"object": "skill.file_change", "type": changeType, "path": filePath, "previous_path": nil, "file": s.presentLocked(filePath)})
		}
	}

	return results, http.StatusOK, "", ""
}

func (s *Server) checkLocked(change map[string]any, filePath string, changeType string) (int, string, string) {
	if filePath == s.FailPath {
		return http.StatusBadRequest, "error", filePath + " was rejected"
	}
	if filePath == "/SKILL.md" && changeType != skillsapi.ChangeTypePut {
		return http.StatusBadRequest, "error", "The root /SKILL.md cannot be deleted or moved"
	}

	ifMatch, hasIfMatch := change["if_match"]
	if !hasIfMatch {
		return http.StatusOK, "", ""
	}

	stored, exists := s.files[filePath]
	if ifMatch == nil && exists {
		return http.StatusPreconditionFailed, "skill_file_exists", filePath + " already exists"
	}
	if _, ok := ifMatch.(string); ok && !exists {
		return http.StatusPreconditionFailed, "skill_file_missing", filePath + " no longer exists"
	}
	if etag, ok := ifMatch.(string); ok && stored.etag != etag {
		return http.StatusPreconditionFailed, "skill_file_changed", filePath + " changed"
	}
	return http.StatusOK, "", ""
}

// Like the real search index, search matches names, never slugs.
func (s *Server) listSkillsLocked(w http.ResponseWriter, query url.Values) {
	search := strings.ToLower(query.Get("search"))
	limit, _ := strconv.Atoi(query.Get("limit"))
	if limit <= 0 {
		limit = 10
	}

	matches := []map[string]any{}
	for _, skill := range s.skills {
		if search == "" || strings.Contains(strings.ToLower(skill["name"].(string)), search) {
			matches = append(matches, skill)
		}
	}

	if after := query.Get("after"); after != "" && !s.IgnoreAfterCursor {
		for index, skill := range matches {
			if skill["id"] == after {
				matches = matches[index+1:]
				break
			}
		}
	}

	hasMore := len(matches) > limit
	if hasMore {
		matches = matches[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": matches, "pagination": map[string]any{"has_more_after": hasMore, "has_more_before": false}})
}

func (s *Server) skill() map[string]any {
	return map[string]any{
		"object": "skill",
		"id":     SkillID,
		"slug":   SkillSlug,
		"name":   "Release Notes",
		"status": "active",
	}
}

func normalize(filePath string) string {
	if strings.HasPrefix(filePath, "/") {
		return filePath
	}
	return "/" + filePath
}

func stringField(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
