package audio

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	// "fmt"  >>> need to add currently off because it is not used in the code
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dhowden/tag"
)

type TrackMeta struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	Album     string `json:"album"`
	Path      string `json:"path"` // relative path inside user's files/
	HasCover  bool   `json:"has_cover"`
	SizeBytes int64  `json:"size_bytes"`
}

type AudioHandler struct {
	dataDir string
}

func NewAudioHandler(dataDir string) *AudioHandler {
	return &AudioHandler{dataDir: dataDir}
}

// ExtractMetadata reads tags from an audio file using tag.ReadFrom (streams header only)
func (h *AudioHandler) ExtractMetadata(username, relPath string) (*TrackMeta, error) {
	fullPath := filepath.Join(h.dataDir, "users", username, "files", relPath)

	file, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		return nil, err
	}

	// Deterministic ID derived from user + relative path
	hasher := sha256.New()
	hasher.Write([]byte(username + ":" + relPath))
	trackID := hex.EncodeToString(hasher.Sum(nil))[:16]

	meta := &TrackMeta{
		ID:        trackID,
		Title:     strings.TrimSuffix(fi.Name(), filepath.Ext(fi.Name())),
		Artist:    "Unknown Artist",
		Album:     "Unknown Album",
		Path:      filepath.ToSlash(relPath),
		HasCover:  false,
		SizeBytes: fi.Size(),
	}

	// Stream ID3/MP4/OGG/FLAC metadata
	m, err := tag.ReadFrom(file)
	if err == nil {
		if strings.TrimSpace(m.Title()) != "" {
			meta.Title = m.Title()
		}
		if strings.TrimSpace(m.Artist()) != "" {
			meta.Artist = m.Artist()
		}
		if strings.TrimSpace(m.Album()) != "" {
			meta.Album = m.Album()
		}

		if pic := m.Picture(); pic != nil && len(pic.Data) > 0 {
			meta.HasCover = true
			h.cacheCoverArt(username, trackID, pic.Data)
		}
	}

	return meta, nil
}

func (h *AudioHandler) cacheCoverArt(username, trackID string, data []byte) {
	thumbDir := filepath.Join(h.dataDir, "users", username, ".thumbs")
	_ = os.MkdirAll(thumbDir, 0755)

	thumbPath := filepath.Join(thumbDir, trackID+".art")
	if _, err := os.Stat(thumbPath); os.IsNotExist(err) {
		_ = os.WriteFile(thumbPath, data, 0644)
	}
}

// HandleAudioLibrary scans user's files for audio tracks, parsing or listing them
// Endpoint: GET /api/audio/library?user=admin&sort=alpha (or sort=artist)
func (h *AudioHandler) HandleAudioLibrary(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("user")
	sortBy := r.URL.Query().Get("sort") // "alpha" or "artist"
	if username == "" {
		http.Error(w, "missing user parameter", http.StatusBadRequest)
		return
	}

	userFilesRoot := filepath.Join(h.dataDir, "users", username, "files")
	var tracks []TrackMeta

	// Walk user's files and detect audio formats without reading entire files
	err := filepath.WalkDir(userFilesRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(d.Name()))
		if isAudioExtension(ext) {
			rel, relErr := filepath.Rel(userFilesRoot, p)
			if relErr == nil {
				track, metaErr := h.ExtractMetadata(username, rel)
				if metaErr == nil {
					tracks = append(tracks, *track)
				}
			}
		}
		return nil
	})

	if err != nil {
		http.Error(w, "failed scanning audio library", http.StatusInternalServerError)
		return
	}

	// In-memory sorting (tracks slice contains only metadata pointers, ~100 bytes each)
	if sortBy == "artist" {
		sort.Slice(tracks, func(i, j int) bool {
			if strings.EqualFold(tracks[i].Artist, tracks[j].Artist) {
				return strings.ToLower(tracks[i].Title) < strings.ToLower(tracks[j].Title)
			}
			return strings.ToLower(tracks[i].Artist) < strings.ToLower(tracks[j].Artist)
		})
	} else {
		// Default: Alphabetical by Title
		sort.Slice(tracks, func(i, j int) bool {
			return strings.ToLower(tracks[i].Title) < strings.ToLower(tracks[j].Title)
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"total":  len(tracks),
		"tracks": tracks,
	})
}

// HandleCoverArt serves cached album art directly
// Endpoint: GET /api/audio/cover?user=admin&id={trackID}
func (h *AudioHandler) HandleCoverArt(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("user")
	trackID := r.URL.Query().Get("id")

	if username == "" || trackID == "" {
		http.Error(w, "missing user or id parameter", http.StatusBadRequest)
		return
	}

	cleanID := filepath.Clean(trackID)
	thumbPath := filepath.Join(h.dataDir, "users", username, ".thumbs", cleanID+".art")

	data, err := os.ReadFile(thumbPath)
	if err != nil {
		http.Error(w, "cover art not found", http.StatusNotFound)
		return
	}

	// Sniff image MIME type (image/jpeg, image/png, etc.)
	mimeType := http.DetectContentType(data)
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func isAudioExtension(ext string) bool {
	switch ext {
	case ".mp3", ".flac", ".m4a", ".aac", ".ogg", ".wav":
		return true
	default:
		return false
	}
}
