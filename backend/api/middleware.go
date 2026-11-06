package api

import (
	"context"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
)

type contextKey string

const (
	SessionIDContextKey contextKey = "session_id"
	UserContextKey      contextKey = "user"
	TokenContextKey     contextKey = "token"
)

type Middleware struct {
	sessionRepo *repository.SessionRepository
	authRepo    repository.AuthRepo
}

func NewMiddleware(sessionRepo *repository.SessionRepository, authRepo repository.AuthRepo) *Middleware {
	return &Middleware{
		sessionRepo: sessionRepo,
		authRepo:    authRepo,
	}
}

func (m *Middleware) CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-ID, X-Auth-Token")
		w.Header().Set("Access-Control-Expose-Headers", "X-Session-ID, Content-Disposition")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Session handles session touch and injection into context.
func (m *Middleware) Session(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.Header.Get("X-Session-ID")
		if sessionID == "" {
			sessionID = r.URL.Query().Get("session_id")
		}

		if sessionID != "" && m.sessionRepo != nil {
			m.sessionRepo.TouchSession(sessionID)
			w.Header().Set("X-Session-ID", sessionID)
			ctx := context.WithValue(r.Context(), SessionIDContextKey, sessionID)
			r = r.WithContext(ctx)
		}

		// Also extract bearer auth token if provided
		tokenStr := ExtractBearerToken(r)
		if tokenStr != "" && m.authRepo != nil {
			user, err := m.authRepo.ValidateToken(r.Context(), tokenStr)
			if err == nil && user != nil {
				ctx := context.WithValue(r.Context(), UserContextKey, user)
				ctx = context.WithValue(ctx, TokenContextKey, tokenStr)
				r = r.WithContext(ctx)
			}
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

// ExtractBearerToken parses Authorization: Bearer <token> or X-Auth-Token or query param token
func ExtractBearerToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
	}
	if tok := r.Header.Get("X-Auth-Token"); tok != "" {
		return tok
	}
	return r.URL.Query().Get("auth_token")
}

// GetUser returns the authenticated User from context if present.
func GetUser(r *http.Request) *models.User {
	if val, ok := r.Context().Value(UserContextKey).(*models.User); ok {
		return val
	}
	return nil
}

// GetToken returns the bearer token string from context if present.
func GetToken(r *http.Request) string {
	if val, ok := r.Context().Value(TokenContextKey).(string); ok {
		return val
	}
	return ExtractBearerToken(r)
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
