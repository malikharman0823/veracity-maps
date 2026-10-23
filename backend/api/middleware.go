package api

import (
	"context"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
)

type contextKey string

const (
	SessionIDContextKey contextKey = "session_id"
)

type Middleware struct {
	repo *repository.SessionRepository
}

func NewMiddleware(repo *repository.SessionRepository) *Middleware {
	return &Middleware{repo: repo}
}

func (m *Middleware) CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-ID")
		w.Header().Set("Access-Control-Expose-Headers", "X-Session-ID, Content-Disposition")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) Session(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.Header.Get("X-Session-ID")
		if sessionID == "" {
			sessionID = r.URL.Query().Get("session_id")
		}

		if sessionID != "" {
			m.repo.TouchSession(sessionID)
			w.Header().Set("X-Session-ID", sessionID)
			ctx := context.WithValue(r.Context(), SessionIDContextKey, sessionID)
			r = r.WithContext(ctx)
		}

		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[PANIC RECOVERED] %s %s: %v\n%s", r.Method, r.URL.Path, rec, string(debug.Stack()))
				models.WriteError(w, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "An unexpected internal server error occurred", nil, nil)
			}
		}()
		start := time.Now()
		next.ServeHTTP(w, r)
		duration := time.Since(start)
		log.Printf("%s %s %s", r.Method, r.URL.Path, duration)
	})
}

func GetSessionID(r *http.Request) string {
	if val, ok := r.Context().Value(SessionIDContextKey).(string); ok && val != "" {
		return val
	}
	if sid := r.Header.Get("X-Session-ID"); sid != "" {
		return sid
	}
	return r.URL.Query().Get("session_id")
}
