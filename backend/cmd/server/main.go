package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	backend "cloud-core"
	"cloud-core/internal/api"
	"cloud-core/internal/users"
)

func main() {
	dataDir := "../data"

	// Fallback check if run directly inside cloud/ root
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		dataDir = "data"
	}

	um, err := users.NewUserManager(dataDir)
	if err != nil {
		log.Fatalf("failed to initialize user manager: %v", err)
	}

	// Loads live files from ../web during development
	webFS := backend.GetWebFileSystem()

	router := api.NewRouter(webFS, dataDir, um)

	srv := &http.Server{
		Addr:         ":9090",
		Handler:      router,
		ReadTimeout:  1 * time.Hour,
		WriteTimeout: 1 * time.Hour,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Println("Cloud Server running at http://0.0.0.0:9999")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down cleanly...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
