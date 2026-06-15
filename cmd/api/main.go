package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ticketing-api/internal/config"
	httpapp "ticketing-api/internal/delivery/http"
	"ticketing-api/internal/delivery/http/handler"
	"ticketing-api/internal/infrastructure/database"
	"ticketing-api/internal/infrastructure/persistence/postgres"
	"ticketing-api/internal/infrastructure/security"
	"ticketing-api/internal/infrastructure/storage"
	"ticketing-api/internal/usecase/auth"
	"ticketing-api/internal/usecase/ticket"
	"ticketing-api/internal/usecase/user"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	db, err := database.Open(database.DefaultOptions(cfg.DatabaseURL))
	if err != nil {
		return err
	}
	defer func() {
		if err := database.Close(db); err != nil {
			log.Printf("close db: %v", err)
		}
	}()

	userRepo := postgres.NewUserRepository(db)
	sessionRepo := postgres.NewAuthSessionRepository(db)
	ticketRepo := postgres.NewTicketRepository(db)
	ticketCategoryRepo := postgres.NewTicketCategoryRepository(db)
	ticketHistoryRepo := postgres.NewTicketHistoryRepository(db)
	ticketCommentRepo := postgres.NewTicketCommentRepository(db)
	ticketAttachmentRepo := postgres.NewTicketAttachmentRepository(db)
	slaPolicyRepo := postgres.NewSLAPolicyRepository(db)
	notificationRepo := postgres.NewNotificationRepository(db)
	txMgr := postgres.NewTxManager(db)

	fileStore, err := storage.NewLocalFileStorage(cfg.UploadLocalDirectory)
	if err != nil {
		return err
	}

	passwordSvc := security.NewBcryptPasswordService(0)
	tokenSvc := security.NewJWTTokenService(cfg.JWTSecret, cfg.JWTIssuer)

	authSvc := auth.NewService(userRepo, sessionRepo, passwordSvc, tokenSvc, cfg.JWTAccessTokenTTL)
	userSvc := user.NewService(userRepo)
	ticketSvc := ticket.NewService(ticketRepo, ticketCategoryRepo, userRepo, ticketHistoryRepo, ticketCommentRepo, txMgr).
		WithPhase5(ticketAttachmentRepo, fileStore, cfg.UploadMaxSizeBytes).
		WithPhase7(slaPolicyRepo, notificationRepo)

	handlers := httpapp.Handlers{
		Health:           handler.NewHealthHandler(),
		Auth:             handler.NewAuthHandler(authSvc),
		User:             handler.NewUserHandler(userSvc),
		Ticket:           handler.NewTicketHandler(ticketSvc),
		TicketCategory:   handler.NewTicketCategoryHandler(ticketSvc),
		TicketComment:    handler.NewTicketCommentHandler(ticketSvc),
		CategoryAdmin:    handler.NewCategoryAdminHandler(ticketSvc),
		Classification:   handler.NewTicketClassificationHandler(ticketSvc),
		TicketAttachment: handler.NewTicketAttachmentHandler(ticketSvc, cfg.UploadMaxSizeBytes),
		Dashboard:        handler.NewDashboardHandler(ticketSvc),
		Notification:     handler.NewNotificationHandler(ticketSvc),
	}

	e := httpapp.NewEcho()
	httpapp.RegisterRoutes(e, handlers, httpapp.RouterDeps{
		Tokens:   tokenSvc,
		Sessions: sessionRepo,
	})

	addr := ":" + cfg.AppPort
	srv := &http.Server{
		Addr:              addr,
		Handler:           e,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("listening on %s (env=%s)", addr, cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case <-quit:
		log.Println("shutting down…")
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	log.Println("server stopped")
	return nil
}
