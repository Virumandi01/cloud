package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud-core/internal/users"
)

type FileItem struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"` // relative to user's root, e.g. "documents/notes.txt"
	IsDir    bool      `json:"is_dir"`
	Size     int64     `json:"size"`
	ModTime  time.Time `json:"mod_time"`
	IsPublic bool      `json:"is_public"`
	Ext      string    `json:"ext"`
}

type FileHandler struct {
	dataDir     string
	userManager *users.UserManager
	accessList  *AccessList
}

func NewFileHandler(dataDir string, um *users.UserManager, al *AccessList) *FileHandler {
	return &FileHandler{
		dataDir:     dataDir,
		userManager: um,
		accessList:  al,
	}
}

// HandleListFiles lists files for a specific directory path inside the user's files/ space
// Endpoint: GET /api/files/list?user=admin&subpath=
func (h *FileHandler) HandleListFiles(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("user")
	subpath := r.URL.Query().Get("subpath")

	if username == "" {
		http.Error(w, "missing user parameter", http.StatusBadRequest)
		return
	}

	cleanSub := filepath.Clean("/" + subpath)
	cleanSub = strings.TrimPrefix(cleanSub, "/")

	// Target directory: data/users/{username}/files/{cleanSub}
	userFilesDir := filepath.Join(h.dataDir, "users", username, "files", cleanSub)

	entries, err := os.ReadDir(userFilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "directory not found", http.StatusNotFound)
			return
		}
		http.Error(w, "unable to read directory", http.StatusInternalServerError)
		return
	}

	items := make([]FileItem, 0, len(entries))

	for _, entry := range entries {
		// Ignore hidden files and chunk caches
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		relPath := filepath.Join(cleanSub, entry.Name())
		// Format in access.txt is: {username}/{relPath}
		fullAccessKey := filepath.ToSlash(filepath.Join(username, relPath))
		isPublic := h.accessList.IsPublic(fullAccessKey)

		ext := ""
		if !entry.IsDir() {
			ext = strings.ToLower(filepath.Ext(entry.Name()))
			ext = strings.TrimPrefix(ext, ".")
		}

		items = append(items, FileItem{
			Name:     entry.Name(),
			Path:     filepath.ToSlash(relPath),
			IsDir:    entry.IsDir(),
			Size:     info.Size(),
			ModTime:  info.ModTime(),
			IsPublic: isPublic,
			Ext:      ext,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"current_path": cleanSub,
		"items":        items,
	})
}

// HandleDeleteFile deletes a file and adjusts the user's allocated storage usage
// Endpoint: POST /api/files/delete (Body: {"user": "admin", "path": "documents/file.pdf"})
func (h *FileHandler) HandleDeleteFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		User string `json:"user"`
		Path string `json:"path"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.User == "" || req.Path == "" {
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return
	}

	cleanPath := filepath.Clean("/" + req.Path)
	cleanPath = strings.TrimPrefix(cleanPath, "/")

	targetPath := filepath.Join(h.dataDir, "users", req.User, "files", cleanPath)
	fi, err := os.Stat(targetPath)
	if err != nil {
		http.Error(w, "file does not exist", http.StatusNotFound)
		return
	}

	fileSize := fi.Size()

	if err := os.RemoveAll(targetPath); err != nil {
		http.Error(w, "failed to delete file", http.StatusInternalServerError)
		return
	}

	// Reclaim quota
	_ = h.userManager.AddUsedBytes(req.User, -fileSize)

	// Remove from access.txt if it was public
	accessKey := filepath.ToSlash(filepath.Join(req.User, cleanPath))
	_ = h.accessList.Remove(accessKey)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "deleted", "reclaimed_bytes": fileSize})
}
