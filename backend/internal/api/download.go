package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type DownloadHandler struct {
	dataDir    string
	accessList *AccessList
}

func NewDownloadHandler(dataDir string, al *AccessList) *DownloadHandler {
	return &DownloadHandler{
		dataDir:    dataDir,
		accessList: al,
	}
}

// ServeHTTP handles HTTP 206 range requests, resumable downloads, and public/private checks
func (h *DownloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	relPath := r.URL.Query().Get("path") // e.g. "admin/files/song.mp3"
	if relPath == "" {
		http.Error(w, "missing path parameter", http.StatusBadRequest)
		return
	}

	// Prevent directory traversal attacks
	cleanRelPath := filepath.Clean(relPath)
	if strings.HasPrefix(cleanRelPath, "..") || strings.HasPrefix(cleanRelPath, "/") {
		http.Error(w, "invalid path traversal", http.StatusForbidden)
		return
	}

	// Security check: Either listed in access.txt (public) OR matches authenticated user
	isPublic := h.accessList.IsPublic(cleanRelPath)
	if !isPublic {
		reqUser := r.Header.Get("X-User")
		if reqUser == "" {
			reqUser = r.URL.Query().Get("user")
		}

		parts := strings.Split(cleanRelPath, string(filepath.Separator))
		if len(parts) == 0 || reqUser == "" || parts[0] != reqUser {
			http.Error(w, "access denied: private file", http.StatusForbidden)
			return
		}
	}

	// Construct absolute path inside data/users/
	fullPath := filepath.Join(h.dataDir, "users", cleanRelPath)
	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		http.Error(w, "unable to open file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		http.Error(w, "failed reading file metadata", http.StatusInternalServerError)
		return
	}

	if fi.IsDir() {
		http.Error(w, "cannot download directory directly", http.StatusBadRequest)
		return
	}

	// Essential headers for resumable downloads and media scrubbing
	w.Header().Set("Accept-Ranges", "bytes")

	// If query has download=true, force attachment prompt; otherwise inline preview
	if r.URL.Query().Get("download") == "true" {
		w.Header().Set("Content-Disposition", "attachment; filename=\""+fi.Name()+"\"")
	}

	http.ServeContent(w, r, fi.Name(), fi.ModTime(), file)
}
