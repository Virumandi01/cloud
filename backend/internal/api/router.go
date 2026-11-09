package api

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"path/filepath"

	"cloud-core/internal/users"
)

type Router struct {
	mux         *http.ServeMux
	userManager *users.UserManager
	accessList  *AccessList
	upload      *UploadHandler
	download    *DownloadHandler
	fileHandler *FileHandler
	dataDir     string
	webFS       fs.FS
}

func NewRouter(webFS fs.FS, dataDir string, um *users.UserManager) http.Handler {
	al := NewAccessList(filepath.Join(dataDir, "access.txt"))
	r := &Router{
		mux:         http.NewServeMux(),
		userManager: um,
		accessList:  al,
		upload:      NewUploadHandler(dataDir, um),
		download:    NewDownloadHandler(dataDir, al),
		fileHandler: NewFileHandler(dataDir, um, al),
		dataDir:     dataDir,
		webFS:       webFS,
	}

	r.registerRoutes()
	return r.mux
}

func (r *Router) registerRoutes() {
	// API Endpoints
	r.mux.HandleFunc("/api/upload/chunk", r.upload.HandleChunk)
	r.mux.Handle("/api/files/download", r.download)
	r.mux.HandleFunc("/api/files/list", r.fileHandler.HandleListFiles)
	r.mux.HandleFunc("/api/files/delete", r.fileHandler.HandleDeleteFile)
	r.mux.HandleFunc("/api/access/toggle", r.handleAccessToggle)
	r.mux.HandleFunc("/api/user/info", r.handleUserInfo)

	// Single-Port Web Asset Serving
	fileServer := http.FileServer(http.FS(r.webFS))
	r.mux.Handle("/", fileServer)
}

func (r *Router) handleAccessToggle(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Path string `json:"path"` // e.g. "admin/sample.mp3"
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil || payload.Path == "" {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	isPublic, err := r.accessList.Toggle(payload.Path)
	if err != nil {
		http.Error(w, "failed to update access list", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"path":      payload.Path,
		"is_public": isPublic,
	})
}

func (r *Router) handleUserInfo(w http.ResponseWriter, req *http.Request) {
	username := req.URL.Query().Get("user")
	if username == "" {
		http.Error(w, "missing user query parameter", http.StatusBadRequest)
		return
	}

	user, exists := r.userManager.GetUser(username)
	if !exists {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"username":            user.Username,
		"storage_limit_bytes": user.StorageLimitBytes,
		"used_bytes":          user.UsedBytes,
		"role":                user.Role,
	})
}
