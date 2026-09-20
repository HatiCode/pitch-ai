package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"

	"pitch-ai/internal/auth"
	"pitch-ai/internal/fixture"
	"pitch-ai/internal/httpapi"
	"pitch-ai/internal/squad"
	firestorestore "pitch-ai/internal/store/firestore"
	"pitch-ai/web"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx := context.Background()

	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		logger.Error("GOOGLE_CLOUD_PROJECT is required")
		os.Exit(1)
	}

	store, err := firestorestore.New(ctx, projectID)
	if err != nil {
		logger.Error("connecting to firestore", "error", err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()

	verifier, err := auth.NewFirebaseVerifier(ctx, projectID)
	if err != nil {
		logger.Error("building token verifier", "error", err)
		os.Exit(1)
	}

	squadService := squad.NewService(store, store, uuid.NewString)
	teamService := squad.NewTeamService(store, store, store, uuid.NewString)
	fixtureService := fixture.NewService(store, store, store, uuid.NewString)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr: ":" + port,
		Handler: httpapi.NewRouter(httpapi.Deps{
			Logger:  logger,
			Assets:  web.Assets(),
			Auth:    auth.Middleware(verifier, store),
			Squad:   squadService,
			Teams:   teamService,
			Fixture: fixtureService,
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		logger.Info("shutdown requested")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
		}
	}()

	logger.Info("listening", "port", port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
	<-shutdownDone
}
