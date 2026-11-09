package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Port           string
	Host           string
	DataDir        string
	ChunkSizeBytes int64
}

// LoadEnv loads key-values from .env if present, checking root and current dirs
func Load() *Config {
	loadDotEnv(".env")
	loadDotEnv("../.env")

	port := getEnv("PORT", "9090")
	host := getEnv("HOST", "0.0.0.0")
	dataDir := getEnv("DATA_DIR", "../data")

	// Adjust dataDir if run directly from repo root vs backend/
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		if _, errRoot := os.Stat("data"); errRoot == nil {
			dataDir = "data"
		}
	}

	chunkSize, _ := strconv.ParseInt(getEnv("CHUNK_SIZE_BYTES", "5242880"), 10, 64)

	return &Config{
		Port:           port,
		Host:           host,
		DataDir:        filepath.Clean(dataDir),
		ChunkSizeBytes: chunkSize,
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
			if os.Getenv(k) == "" {
				_ = os.Setenv(k, v)
			}
		}
	}
}
