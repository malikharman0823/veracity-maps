package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/malikharman0823/veracity-maps/api"
	"github.com/malikharman0823/veracity-maps/config"
	"github.com/malikharman0823/veracity-maps/functions/datasets"
	"github.com/malikharman0823/veracity-maps/functions/export"
	"github.com/malikharman0823/veracity-maps/functions/georeference"
	"github.com/malikharman0823/veracity-maps/functions/upload"
	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
	"github.com/malikharman0823/veracity-maps/storage"
)

func main() {
	cfg := config.LoadConfig()

	sm, err := storage.NewStorageManager(cfg.SessionStorageDir, cfg.UploadDir, cfg.ProcessedDir)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	repo := repository.NewSessionRepository()
	repo.SetStorageDir(cfg.SessionStorageDir)
	middleware := api.NewMiddleware(repo)

	datasetService := services.NewDatasetService(cfg, repo, sm)
	georefService := services.NewGeoreferenceService(repo, sm)
	exportService := services.NewExportService(repo, sm)

	uploadHandler := upload.NewHandler(datasetService, repo)
	datasetsHandler := datasets.NewHandler(datasetService, repo)
	georefHandler := georeference.NewHandler(georefService, repo)
	exportHandler := export.NewHandler(exportService, repo)

	router := api.NewRouter(
		repo,
		middleware,
		uploadHandler,
		datasetsHandler,
		georefHandler,
		exportHandler,
	)

	// Background session cleaner
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			removed := repo.CleanupExpiredSessions(cfg.SessionIdleTimeout, cfg.SessionMaxLifetime, func(sessionID string) {
				sessionDir := sm.GetSessionDir(sessionID)
				_ = storage.SafeRemoveAll(sessionDir)
			})
			if len(removed) > 0 {
				log.Printf("Cleaned up %d expired sessions", len(removed))
			}
		}
	}()

	addr := fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      router.BuildHandler(),
		ReadTimeout:  10 * time.Minute,
		WriteTimeout: 10 * time.Minute,
	}

	gdal := gis.DetectGDAL()
	log.Printf("==================================================")
	log.Printf(" Veracity Maps Backend starting on http://localhost:%s", cfg.Port)
	if gdal.Available {
		log.Printf(" GDAL Status: Available (%s)", gdal.VersionMessage)
	} else {
		log.Printf(" GDAL Status: %s", gdal.VersionMessage)
	}
	log.Printf(" Sessions Dir: %s", cfg.SessionStorageDir)
	log.Printf("==================================================")

	// Graceful shutdown handling
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to listen: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down server gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
	log.Println("Server stopped")
}
