package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	backend "cloud-core"
	"cloud-core/internal/api"
	"cloud-core/internal/config"
	"cloud-core/internal/users"
)

func main() {
	// 1. Ultra-low RAM tuning:
	// Set aggressive GC threshold (triggers GC when heap grows 20% over live data instead of default 100%)
	debug.SetGCPercent(20)
	// Suggest a soft memory ceiling to the Go runtime (e.g. 50MB)
	debug.SetMemoryLimit(50 * 1024 * 1024)

	// 2. Load Dynamic .env Configuration
	cfg := config.Load()

	// 3. Initialize Core Authority Services
	um, err := users.NewUserManager(cfg.DataDir)
	if err != nil {
		log.Fatalf("failed to initialize user manager: %v", err)
	}

	// 4. Mount Embedded/Live Web Asset FS
	webFS := backend.GetWebFileSystem()

	// 5. Create Master Router
	router := api.NewRouter(webFS, cfg.DataDir, um)

	bindAddr := fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)

	srv := &http.Server{
		Addr:         bindAddr,
		Handler:      router,
		ReadTimeout:  1 * time.Hour,
		WriteTimeout: 1 * time.Hour,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Cloud Daemon live on http://%s (Data Root: %s)\n", bindAddr, cfg.DataDir)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen error: %v", err)
		}
	}()

	// Graceful Shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down cleanly...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
