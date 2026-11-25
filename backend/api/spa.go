package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ResolveFrontendDistDir returns the directory containing the built React SPA
// (index.html + assets/) if available, or "" when running without a built UI.
func ResolveFrontendDistDir() string {
	candidates := []string{}
	if envDir := strings.TrimSpace(os.Getenv("FRONTEND_DIST_DIR")); envDir != "" {
		candidates = append(candidates, envDir)
	}
	candidates = append(candidates,
		filepath.Join("..", "frontend", "dist"),
		filepath.Join("frontend", "dist"),
		"dist",
	)
	for _, dir := range candidates {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		if info, err := os.Stat(filepath.Join(absDir, "index.html")); err == nil && !info.IsDir() {
			return absDir
		}
	}
	return ""
}

// isBackendAPIPrefix reports whether path belongs to a backend API/tile route
// that should return JSON 404 rather than the React SPA index.html.
func isBackendAPIPrefix(path string) bool {
	if path == "" || path == "/" {
		return false
	}
	prefixes := []string{
		"/api/",
		"/tiles/",
		"/datasets/",
		"/upload",
		"/documents/",
		"/buildings/",
		"/georeference/",
		"/export/",
		"/sessions",
		"/health",
		"/static/",
		"/auth/",
		"/profile",
		"/projects",
		"/layers/",
		"/jobs",
		"/processing/",
		"/analysis/",
		"/workspace-registry/",
	}
	for _, p := range prefixes {
		if path == strings.TrimSuffix(p, "/") || strings.HasPrefix(path, p) {
			return true
		}
	}
	// /maps/pub_.../download/... or /maps/.../style.json are backend endpoints
	if strings.HasPrefix(path, "/maps/") && (strings.Contains(path, "/download/") || strings.HasSuffix(path, "/style.json") || strings.Contains(path, "/layers")) {
		return true
	}
	return false
}

// serveSPA serves static files from the built frontend dist directory and falls
// back to index.html for client-side React Router paths. Returns true if handled.
func serveSPA(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if isBackendAPIPrefix(r.URL.Path) {
		return false
	}
	distDir := ResolveFrontendDistDir()
	if distDir == "" {
		return false
	}

	cleanRel := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if cleanRel == "." {
		cleanRel = ""
	}
	if cleanRel != "" && !strings.HasPrefix(cleanRel, "..") {
		candidate := filepath.Join(distDir, cleanRel)
		absCandidate, err := filepath.Abs(candidate)
		if err == nil && (absCandidate == distDir || strings.HasPrefix(absCandidate, distDir+string(filepath.Separator))) {
			if info, err := os.Stat(absCandidate); err == nil && !info.IsDir() {
				http.ServeFile(w, r, absCandidate)
				return true
			}
		}
		// Missing static asset (e.g. .js/.css/.png/.map) should not return index.html
		if strings.Contains(filepath.Base(cleanRel), ".") {
			return false
		}
	}

	indexPath := filepath.Join(distDir, "index.html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeFile(w, r, indexPath)
	return true
}
