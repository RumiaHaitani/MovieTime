package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"movietime-backend/internal/config"
	"movietime-backend/internal/middleware"
	"movietime-backend/internal/room"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func main() {
	cfg := config.Load()

	// In-memory хранилище (без БД)
	store := room.NewRoomStore()
	roomSvc := room.NewService(store)

	restHandler := room.NewRestHandler(roomSvc)
	wsHandler := room.NewWebSocketHandler(roomSvc)

	r := chi.NewRouter()
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(middleware.LoggingMiddleware)
	r.Use(middleware.RequestTimeoutMiddleware(15 * time.Second))

	// REST API
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.JSONHeaderMiddleware)
		r.Mount("/", restHandler.Router())
	})

	// WebSocket
	r.Get("/ws", wsHandler.Handle)

	// Статические файлы фронтенда
	r.Handle("/*", http.FileServer(http.Dir("./static")))

	// HTTP сервер
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return appCtx },
	}

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		log.Fatalf("listen error: %v", err)
	}

	log.Printf("Server running on port %s (http://localhost:8080/)", cfg.Port)

	serverErrCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
		}
		close(serverErrCh)
	}()

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-sigCtx.Done():
		log.Println("shutdown signal received")
	case err := <-serverErrCh:
		if err != nil {
			log.Printf("server error: %v", err)
		}
	}

	appCancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
		srv.Close()
	}

	log.Println("server stopped")
}
