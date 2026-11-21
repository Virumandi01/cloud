package archive

import (
	"archive/zip"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type ZipItem struct {
	Name             string `json:"name"`
	IsDir            bool   `json:"is_dir"`
	UncompressedSize uint64 `json:"uncompressed_size"`
}

type ArchiveHandler struct {
	dataDir string
}

func NewArchiveHandler(dataDir string) *ArchiveHandler {
	return &ArchiveHandler{dataDir: dataDir}
}

// HandleInspectZip reads the zip central directory table without extracting to disk
// Endpoint: GET /api/archive/inspect?user=admin&path=archive.zip
func (h *ArchiveHandler) HandleInspectZip(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("user")
	relPath := r.URL.Query().Get("path")

	if username == "" || relPath == "" {
		http.Error(w, "missing parameters", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(h.dataDir, "users", username, "files", filepath.Clean(relPath))
	rdr, err := zip.OpenReader(fullPath)
	if err != nil {
		http.Error(w, "failed to read zip file", http.StatusBadRequest)
		return
	}
	defer rdr.Close()

	var files []ZipItem
	for _, f := range rdr.File {
		files = append(files, ZipItem{
			Name:             f.Name,
			IsDir:            f.FileInfo().IsDir(),
			UncompressedSize: f.UncompressedSize64,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"archive": relPath,
		"entries": files,
	})
}

// HandleUnzip extracts the zip directly into a destination directory
// Endpoint: POST /api/archive/unzip (Body: {"user":"admin", "path":"archive.zip", "dest":"extracted_folder"})
func (h *ArchiveHandler) HandleUnzip(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		User string `json:"user"`
		Path string `json:"path"`
		Dest string `json:"dest"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.User == "" || req.Path == "" {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	srcZip := filepath.Join(h.dataDir, "users", req.User, "files", filepath.Clean(req.Path))
	destDir := filepath.Join(h.dataDir, "users", req.User, "files", filepath.Clean(req.Dest))

	rdr, err := zip.OpenReader(srcZip)
	if err != nil {
		http.Error(w, "unable to open archive", http.StatusBadRequest)
		return
	}
	defer rdr.Close()

	for _, f := range rdr.File {
		targetPath := filepath.Join(destDir, f.Name)

		// Prevent zip-slip vulnerability
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(destDir)) {
			continue
		}

		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(targetPath, 0755)
			continue
		}

		_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
		outFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			continue
		}

		rc, err := f.Open()
		if err == nil {
			_, _ = io.Copy(outFile, rc)
			rc.Close()
		}
		outFile.Close()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "unzipped"})
}
