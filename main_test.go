package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func uploadRequest(t *testing.T, filename, content, token, query string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/upload"+query, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func TestUpload(t *testing.T) {
	dir := t.TempDir()
	s := &server{uploadDir: dir, token: "secret", maxBytes: 1024}
	recorder := httptest.NewRecorder()
	s.authorize(s.upload)(recorder, uploadRequest(t, "hello.txt", "hello", "secret", ""))

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	content, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if err != nil || string(content) != "hello" {
		t.Fatalf("saved content = %q, err = %v", content, err)
	}
}

func TestUploadRequiresToken(t *testing.T) {
	s := &server{uploadDir: t.TempDir(), token: "secret", maxBytes: 1024}
	recorder := httptest.NewRecorder()
	s.authorize(s.upload)(recorder, uploadRequest(t, "hello.txt", "hello", "", ""))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestUploadDoesNotOverwriteByDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &server{uploadDir: dir, token: "secret", maxBytes: 1024}
	recorder := httptest.NewRecorder()
	s.upload(recorder, uploadRequest(t, "hello.txt", "new", "secret", ""))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestUploadStripsClientPath(t *testing.T) {
	dir := t.TempDir()
	s := &server{uploadDir: dir, token: "secret", maxBytes: 1024}
	recorder := httptest.NewRecorder()
	s.upload(recorder, uploadRequest(t, "../escape.txt", "safe", "secret", ""))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response["filename"] != "escape.txt" {
		t.Fatalf("response = %#v, err = %v", response, err)
	}
}
