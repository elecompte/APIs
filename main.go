package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type server struct {
	uploadDir string
	token     string
	maxBytes  int64
	listenarrURL string
	listenarrKey string
}

type errorResponse struct {
	Error string `json:"error"`
}

func main() {
	token := os.Getenv("API_TOKEN")
	if token == "" {
		log.Fatal("API_TOKEN must be set")
	}

	maxMB := int64(100)
	if raw := os.Getenv("MAX_UPLOAD_MB"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 {
			log.Fatal("MAX_UPLOAD_MB must be a positive integer")
		}
		maxMB = parsed
	}

	s := &server{
		uploadDir: envOr("UPLOAD_DIR", "/uploads"),
		token:     token,
		maxBytes:  maxMB * 1024 * 1024,
		listenarrURL: os.Getenv("LISTENARR_URL"),
		listenarrKey: os.Getenv("LISTENARR_API_KEY"),
	}
	if err := os.MkdirAll(s.uploadDir, 0o755); err != nil {
		log.Fatalf("create upload directory: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /upload", s.authorize(s.upload))
	mux.HandleFunc("POST /goodreads/request", s.authorize(s.goodreadsRequest))

	addr := envOr("LISTEN_ADDR", ":8080")
	log.Printf("upload API listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func (s *server) authorize(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, prefix) || subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(header, prefix)), []byte(s.token)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, http.StatusUnauthorized, "missing or invalid bearer token")
			return
		}
		next(w, r)
	}
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.maxBytes+1024*1024)
	if err := r.ParseMultipartForm(s.maxBytes); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("invalid upload or file exceeds %d MB", s.maxBytes/(1024*1024)))
		return
	}

	source, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "multipart field 'file' is required")
		return
	}
	defer source.Close()

	name, err := safeFilename(header.Filename)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	destination := filepath.Join(s.uploadDir, name)
	overwrite := r.URL.Query().Get("overwrite") == "true"
	target, err := os.CreateTemp(s.uploadDir, ".upload-*")
	if err != nil {
		log.Printf("create temporary upload: %v", err)
		writeError(w, http.StatusInternalServerError, "could not create temporary file")
		return
	}
	temporary := target.Name()
	keepTemporary := true
	defer func() {
		if keepTemporary {
			if removeErr := os.Remove(temporary); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				log.Printf("remove temporary upload: %v", removeErr)
			}
		}
	}()

	written, copyErr := io.Copy(target, io.LimitReader(source, s.maxBytes+1))
	chmodErr := target.Chmod(0o644)
	closeErr := target.Close()
	if copyErr != nil || chmodErr != nil || closeErr != nil || written > s.maxBytes {
		if written > s.maxBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "file exceeds upload limit")
		} else {
			writeError(w, http.StatusInternalServerError, "upload failed")
		}
		return
	}

	if overwrite {
		err = os.Rename(temporary, destination)
	} else {
		err = os.Link(temporary, destination)
	}
	if errors.Is(err, os.ErrExist) {
		writeError(w, http.StatusConflict, "a file with that name already exists; add ?overwrite=true to replace it")
		return
	}
	if err != nil {
		log.Printf("publish upload: %v", err)
		writeError(w, http.StatusInternalServerError, "could not publish uploaded file")
		return
	}
	if !overwrite {
		if err := os.Remove(temporary); err != nil {
			log.Printf("remove linked temporary upload: %v", err)
		}
	}
	keepTemporary = false

	writeJSON(w, http.StatusCreated, map[string]any{
		"filename": name,
		"bytes":    written,
	})
}

func safeFilename(raw string) (string, error) {
	name := filepath.Base(strings.ReplaceAll(raw, "\\", "/"))
	if name == "." || name == "" || name == ".." || strings.ContainsRune(name, '\x00') {
		return "", errors.New("invalid filename")
	}
	return name, nil
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write response: %v", err)
	}
}
