package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"cloud-core/internal/users"
)

type UploadHandler struct {
	dataDir     string
	userManager *users.UserManager
}

func NewUploadHandler(dataDir string, um *users.UserManager) *UploadHandler {
	return &UploadHandler{dataDir: dataDir, userManager: um}
}

func (h *UploadHandler) HandleChunk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username := r.Header.Get("X-User")
	uploadID := r.Header.Get("X-Upload-ID")
	chunkIdxStr := r.Header.Get("X-Chunk-Index")
	totalChunksStr := r.Header.Get("X-Total-Chunks")
	expectedHash := r.Header.Get("X-Chunk-SHA256")
	fileName := r.Header.Get("X-File-Name")

	if username == "" || uploadID == "" || chunkIdxStr == "" || totalChunksStr == "" || fileName == "" {
		http.Error(w, "missing required chunk headers", http.StatusBadRequest)
		return
	}

	chunkIdx, _ := strconv.Atoi(chunkIdxStr)
	totalChunks, _ := strconv.Atoi(totalChunksStr)

	// Validate quota before accepting body
	if err := h.userManager.CheckQuota(username, r.ContentLength); err != nil {
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		return
	}

	chunkSessionDir := filepath.Join(h.dataDir, "users", username, ".chunks", uploadID)
	_ = os.MkdirAll(chunkSessionDir, 0755)

	chunkPath := filepath.Join(chunkSessionDir, fmt.Sprintf("part_%05d", chunkIdx))
	outFile, err := os.Create(chunkPath)
	if err != nil {
		http.Error(w, "unable to allocate chunk file", http.StatusInternalServerError)
		return
	}
	defer outFile.Close()

	hasher := sha256.New()
	reader := io.TeeReader(r.Body, hasher)

	written, err := io.Copy(outFile, reader)
	if err != nil {
		http.Error(w, "failed to stream chunk data", http.StatusInternalServerError)
		return
	}

	calculatedHash := hex.EncodeToString(hasher.Sum(nil))
	if expectedHash != "" && calculatedHash != expectedHash {
		_ = os.Remove(chunkPath)
		http.Error(w, "SHA-256 checksum mismatch", http.StatusUnprocessableEntity)
		return
	}

	// Final chunk received: assemble the complete file
	if chunkIdx == totalChunks-1 {
		go h.assembleFile(username, uploadID, fileName, totalChunks)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"chunk":   chunkIdx,
		"written": written,
		"sha256":  calculatedHash,
	})
}

func (h *UploadHandler) assembleFile(username, uploadID, fileName string, totalChunks int) {
	sessionDir := filepath.Join(h.dataDir, "users", username, ".chunks", uploadID)
	finalTarget := filepath.Join(h.dataDir, "users", username, "files", fileName)

	out, err := os.Create(finalTarget)
	if err != nil {
		return
	}
	defer out.Close()

	hasher := sha256.New()
	mw := io.MultiWriter(out, hasher)

	var totalSize int64
	for i := 0; i < totalChunks; i++ {
		p := filepath.Join(sessionDir, fmt.Sprintf("part_%05d", i))
		pf, err := os.Open(p)
		if err != nil {
			return
		}
		n, _ := io.Copy(mw, pf)
		totalSize += n
		pf.Close()
	}

	_ = h.userManager.AddUsedBytes(username, totalSize)
	_ = os.RemoveAll(sessionDir)
}
