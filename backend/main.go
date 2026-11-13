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
	"github.com/malikharman0823/veracity-maps/db"
	"github.com/malikharman0823/veracity-maps/functions/auth"
	"github.com/malikharman0823/veracity-maps/functions/datasets"
	"github.com/malikharman0823/veracity-maps/functions/export"
	"github.com/malikharman0823/veracity-maps/functions/georeference"
	"github.com/malikharman0823/veracity-maps/functions/maps"
	"github.com/malikharman0823/veracity-maps/functions/profile"
	"github.com/malikharman0823/veracity-maps/functions/projects"
	"github.com/malikharman0823/veracity-maps/functions/upload"
	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
	"github.com/malikharman0823/veracity-maps/storage"
)

func main() {
	cfg := config.LoadConfig()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}
	dbTimeout := 1200 * time.Millisecond
	if cfg.Environment == "production" {
		dbTimeout = 5 * time.Second
	}
	startupCtx, cancelStartup := context.WithTimeout(context.Background(), dbTimeout)
	sqlDB, err := db.InitDB(startupCtx, cfg.MSSQLConnStr)

	var authRepo repository.AuthRepo
	var otpRepo repository.OTPRepo
	var profileRepo repository.ProfileRepo
	var mapRepo repository.MapRepo

	if err != nil {
		cancelStartup()
		if cfg.Environment == "production" {
			log.Fatal("Failed to connect to MSSQL; durable authentication and map persistence are required (connection details withheld)")
		}
		log.Println("==================================================================")
		log.Println(" [DEV NOTICE] MSSQL Server TCP connection (port 1433) is offline.")
		log.Println(" [DEV NOTICE] Automatically running in local development mode.")
		log.Println(" [DEV NOTICE] SMTP Email OTP delivery and personal maps are ACTIVE.")
		log.Println("==================================================================")

		memAuth, authErr := repository.NewAuthRepository("storage")
		if authErr != nil {
			log.Fatalf("Failed to initialize auth repository: %v", authErr)
		}
		authRepo = memAuth
		otpRepo = repository.NewMemoryOTPRepository()
		profileRepo = repository.NewMemoryProfileRepository(memAuth)
		mapRepo = repository.NewMemoryMapRepository()
	} else {
		defer sqlDB.Close()
		if err := db.RunMigrations(startupCtx, sqlDB, "migrations"); err != nil {
			cancelStartup()
			log.Fatalf("Failed to migrate MSSQL schema: %v", err)
		}
		cancelStartup()
		authRepo = repository.NewMSSQLAuthRepository(sqlDB)
		otpRepo = repository.NewMSSQLOTPRepository(sqlDB)
		profileRepo = repository.NewMSSQLProfileRepository(sqlDB)
		mapRepo = repository.NewMSSQLMapRepository(sqlDB)
	}

	sm, err := storage.NewStorageManager(cfg.SessionStorageDir, cfg.UploadDir, cfg.ProcessedDir)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	repo := repository.NewSessionRepository()
	repo.SetStorageDir(cfg.SessionStorageDir)

	projRepo, err := repository.NewProjectRepository("storage")
	if err != nil {
		log.Fatalf("Failed to initialize project repository: %v", err)
	}
	if sqlDB != nil {
		projRepo.SetDB(sqlDB)
	}

	middleware := api.NewMiddleware(repo, authRepo)

	datasetService := services.NewDatasetService(cfg, repo, sm)
	georefService := services.NewGeoreferenceService(repo, sm)
	exportService := services.NewExportService(repo, sm)

	uploadHandler := upload.NewHandler(datasetService, repo, sm)
	uploadHandler.SetMapRepo(mapRepo)
	datasetsHandler := datasets.NewHandler(datasetService, repo)
	georefHandler := georeference.NewHandler(georefService, repo)
	exportHandler := export.NewHandler(exportService, repo)
	var mailer services.Mailer
	if cfg.SMTPHost != "" {
		mailer = services.NewSMTPMailer(services.SMTPConfig{
			Host:          cfg.SMTPHost,
			Port:          cfg.SMTPPort,
			Username:      cfg.SMTPUsername,
			Password:      cfg.SMTPPassword,
			From:          cfg.SMTPFrom,
			UseTLS:        cfg.SMTPUseTLS,
			SkipTLSVerify: cfg.Environment != "production",
		})
	} else if cfg.ResendAPIKey != "" {
		mailer = services.NewResendMailer(cfg.ResendAPIKey, cfg.ResendFrom)
	}
	authHandler := auth.NewHandlerWithOTP(authRepo, otpRepo, profileRepo, mailer, cfg.AppPublicURL)
	projectsHandler := projects.NewHandler(projRepo, repo, authRepo, mailer, cfg.AppPublicURL)
	profileHandler := profile.NewHandler(profileRepo, mailer, "storage", cfg.AppPublicURL)
	profileHandler.SetRepositories(repo, projRepo, authRepo)
	mapsHandler := maps.NewHandler(mapRepo, mailer, cfg.AppPublicURL)
	mapsHandler.SetConfigurationService(services.NewMapConfigurationService(mapRepo, cfg.UploadDir, cfg.SessionStorageDir))

	router := api.NewRouter(
		repo,
		authRepo,
		projRepo,
		middleware,
		uploadHandler,
		datasetsHandler,
		georefHandler,
		exportHandler,
		authHandler,
		projectsHandler,
		profileHandler,
	)
	router.SetMapsHandler(mapsHandler)

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
