package bridge

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSlackThreadAPI_GetMessages_MissingParams(t *testing.T) {
	api := NewSlackThreadAPI(nil, slog.Default())
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	tests := []struct {
		name string
		url  string
	}{
		{"missing both", "/api/slack/threads"},
		{"missing ts", "/api/slack/threads?channel=C-test"},
		{"missing channel", "/api/slack/threads?ts=1111.2222"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("got status %d, want %d", w.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestSlackThreadAPI_GetMessages_MethodNotAllowed(t *testing.T) {
	api := NewSlackThreadAPI(nil, slog.Default())
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/slack/threads?channel=C-test&ts=1.1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("got status %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestSlackThreadAPI_PostReply_MissingFields(t *testing.T) {
	api := NewSlackThreadAPI(nil, slog.Default())
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	tests := []struct {
		name string
		body replyRequest
	}{
		{"missing channel", replyRequest{ThreadTS: "1.1", Text: "hello"}},
		{"missing thread_ts", replyRequest{Channel: "C-test", Text: "hello"}},
		{"missing text", replyRequest{Channel: "C-test", ThreadTS: "1.1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/slack/threads/reply", bytes.NewReader(body))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("got status %d, want %d", w.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestSlackThreadAPI_PostReply_MethodNotAllowed(t *testing.T) {
	api := NewSlackThreadAPI(nil, slog.Default())
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/slack/threads/reply", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("got status %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestSlackThreadAPI_PostReply_InvalidBody(t *testing.T) {
	api := NewSlackThreadAPI(nil, slog.Default())
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/slack/threads/reply",
		bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSlackThreadAPI_FileProxy_MissingFileID(t *testing.T) {
	api := NewSlackThreadAPI(nil, slog.Default())
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/slack/files/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSlackThreadAPI_FileProxy_MethodNotAllowed(t *testing.T) {
	api := NewSlackThreadAPI(nil, slog.Default())
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/slack/files/F12345", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("got status %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestSlackFileInfo_JSON(t *testing.T) {
	info := SlackFileInfo{
		ID:       "F12345",
		Name:     "screenshot.png",
		Mimetype: "image/png",
		Size:     1024,
		ProxyURL: "/api/slack/files/F12345",
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded SlackFileInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ID != info.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, info.ID)
	}
	if decoded.ProxyURL != info.ProxyURL {
		t.Errorf("ProxyURL = %q, want %q", decoded.ProxyURL, info.ProxyURL)
	}
}

func TestThreadMessage_WithFiles(t *testing.T) {
	msg := ThreadMessage{
		Author:    "U-TEST",
		Text:      "check this screenshot",
		Timestamp: "1111.2222",
		Files: []SlackFileInfo{
			{
				ID:       "F12345",
				Name:     "screenshot.png",
				Mimetype: "image/png",
				Size:     2048,
				ProxyURL: "/api/slack/files/F12345",
			},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ThreadMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(decoded.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(decoded.Files))
	}
	if decoded.Files[0].Name != "screenshot.png" {
		t.Errorf("file name = %q, want %q", decoded.Files[0].Name, "screenshot.png")
	}
}

func TestThreadMessage_WithoutFiles_OmitsField(t *testing.T) {
	msg := ThreadMessage{
		Author:    "U-TEST",
		Text:      "no files here",
		Timestamp: "1111.2222",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Verify "files" key is omitted (omitempty).
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if _, ok := raw["files"]; ok {
		t.Error("expected 'files' to be omitted when empty")
	}
}
