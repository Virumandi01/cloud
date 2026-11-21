package api

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"cloud-core/internal/auth"
	"cloud-core/internal/extensions/archive"
	"cloud-core/internal/extensions/audio"
	"cloud-core/internal/users"
)

type Router struct {
	mux            *http.ServeMux
	userManager    *users.UserManager
	sessions       *auth.SessionStore
	accessList     *AccessList
	upload         *UploadHandler
	download       *DownloadHandler
	fileHandler    *FileHandler
	audioHandler   *audio.AudioHandler
	archiveHandler *archive.ArchiveHandler
	dataDir        string
	webFS          fs.FS
}

func NewRouter(webFS fs.FS, dataDir string, um *users.UserManager) http.Handler {
	al := NewAccessList(filepath.Join(dataDir, "access.txt"))
	r := &Router{
		mux:            http.NewServeMux(),
		userManager:    um,
		sessions:       auth.NewSessionStore(),
		accessList:     al,
		upload:         NewUploadHandler(dataDir, um),
		download:       NewDownloadHandler(dataDir, al),
		fileHandler:    NewFileHandler(dataDir, um, al),
		audioHandler:   audio.NewAudioHandler(dataDir),
		archiveHandler: archive.NewArchiveHandler(dataDir),
		dataDir:        dataDir,
		webFS:          webFS,
	}

	r.registerRoutes()
	return r.mux
}

func (r *Router) registerRoutes() {
	// Auth & Profile
	r.mux.HandleFunc("/api/auth/login", r.handleLogin)
	r.mux.HandleFunc("/api/auth/logout", r.handleLogout)
	r.mux.HandleFunc("/api/auth/me", r.handleMe)
	r.mux.HandleFunc("/api/user/wallpaper", r.handleWallpaper)
	r.mux.HandleFunc("/api/user/info", r.handleUserInfo)

	// Admin Console
	r.mux.HandleFunc("/api/admin/users", r.handleAdminUsers)

	// Files & Access
	r.mux.HandleFunc("/api/upload/chunk", r.upload.HandleChunk)
	r.mux.Handle("/api/files/download", r.download)
	r.mux.HandleFunc("/api/files/list", r.fileHandler.HandleListFiles)
	r.mux.HandleFunc("/api/files/delete", r.fileHandler.HandleDeleteFile)
	r.mux.HandleFunc("/api/files/mkdir", r.handleMkdir)
	r.mux.HandleFunc("/api/access/toggle", r.handleAccessToggle)

	// Audio & Archives
	r.mux.HandleFunc("/api/audio/library", r.audioHandler.HandleAudioLibrary)
	r.mux.HandleFunc("/api/audio/cover", r.audioHandler.HandleCoverArt)
	r.mux.HandleFunc("/api/archive/inspect", r.archiveHandler.HandleInspectZip)
	r.mux.HandleFunc("/api/archive/unzip", r.archiveHandler.HandleUnzip)

	// Single-Port Web Asset Serving
	fileServer := http.FileServer(http.FS(r.webFS))
	r.mux.Handle("/", fileServer)
}

func (r *Router) handleLogin(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var creds struct {
		Username string `json:"username"`
		Passcode string `json:"passcode"`
	}
	if err := json.NewDecoder(req.Body).Decode(&creds); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	user, err := r.userManager.Authenticate(creds.Username, creds.Passcode)
	if err != nil {
		http.Error(w, "invalid username or passcode", http.StatusUnauthorized)
		return
	}

	token := r.sessions.Create(user.Username)
	http.SetCookie(w, &http.Cookie{
		Name:     "cloud_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"username":            user.Username,
		"role":                user.Role,
		"storage_limit_bytes": user.StorageLimitBytes,
		"used_bytes":          user.UsedBytes,
	})
}

func (r *Router) handleLogout(w http.ResponseWriter, req *http.Request) {
	r.sessions.Delete(req)
	http.SetCookie(w, &http.Cookie{
		Name:   "cloud_session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	w.WriteHeader(http.StatusOK)
}

func (r *Router) handleMe(w http.ResponseWriter, req *http.Request) {
	username, ok := r.sessions.GetUser(req)
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
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
		"role":                user.Role,
		"storage_limit_bytes": user.StorageLimitBytes,
		"used_bytes":          user.UsedBytes,
	})
}

func (r *Router) handleWallpaper(w http.ResponseWriter, req *http.Request) {
	username, ok := r.sessions.GetUser(req)
	if !ok {
		username = "admin"
	}

	wpPath := filepath.Join(r.dataDir, "users", username, "wallpaper.img")

	if req.Method == http.MethodPost {
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		req.Body = http.MaxBytesReader(w, req.Body, 15<<20)
		out, err := os.Create(wpPath)
		if err != nil {
			http.Error(w, "failed saving wallpaper", http.StatusInternalServerError)
			return
		}
		defer out.Close()
		_, _ = io.Copy(out, req.Body)
		w.WriteHeader(http.StatusOK)
		return
	}

	if _, err := os.Stat(wpPath); os.IsNotExist(err) {
		http.NotFound(w, req)
		return
	}
	http.ServeFile(w, req, wpPath)
}

func (r *Router) handleMkdir(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		User string `json:"user"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.User == "" || body.Path == "" {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	clean := strings.TrimPrefix(filepath.Clean("/"+body.Path), "/")
	target := filepath.Join(r.dataDir, "users", body.User, "files", clean)
	if err := os.MkdirAll(target, 0755); err != nil {
		http.Error(w, "failed creating folder", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (r *Router) handleAdminUsers(w http.ResponseWriter, req *http.Request) {
	if _, isAdmin := r.sessions.RequireAdmin(req, r.userManager); !isAdmin {
		http.Error(w, "admin access required", http.StatusForbidden)
		return
	}

	if req.Method == http.MethodGet {
		list := r.userManager.ListAllUsers()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
		return
	}

	if req.Method == http.MethodPost {
		var payload struct {
			Action            string `json:"action"`
			Username          string `json:"username"`
			Passcode          string `json:"passcode"`
			StorageLimitBytes int64  `json:"storage_limit_bytes"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil || payload.Username == "" {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}

		if payload.Action == "create" {
			err := r.userManager.CreateUser(payload.Username, payload.Passcode, payload.StorageLimitBytes)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		} else {
			if payload.StorageLimitBytes > 0 {
				_ = r.userManager.SetUserQuota(payload.Username, payload.StorageLimitBytes)
			}
			if payload.Passcode != "" {
				_ = r.userManager.SetUserPasscode(payload.Username, payload.Passcode)
			}
		}
		w.WriteHeader(http.StatusOK)
	}
}

func (r *Router) handleAccessToggle(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Path string `json:"path"`
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
