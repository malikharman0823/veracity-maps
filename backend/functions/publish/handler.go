package publish

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/services"
)

type Handler struct {
	svc          *services.PublishService
	appPublicURL string
}

func NewHandler(svc *services.PublishService, appPublicURL string) *Handler {
	return &Handler{
		svc:          svc,
		appPublicURL: strings.TrimSuffix(strings.TrimSpace(appPublicURL), "/"),
	}
}

func (h *Handler) GetService() *services.PublishService {
	return h.svc
}

func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "localhost:8000"
	}
	return scheme + "://" + host
}

func writePublishErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, models.ErrPublishedMapNotFound):
		models.WriteError(w, http.StatusNotFound, "PUBLISHED_MAP_NOT_FOUND", "Published map not found", nil, nil)
	case errors.Is(err, models.ErrPublishedMapRevoked):
		models.WriteError(w, http.StatusNotFound, "PUBLISHED_MAP_REVOKED", "This published map link has been revoked", nil, nil)
	case errors.Is(err, models.ErrPublishedMapAccessDenied):
		models.WriteError(w, http.StatusForbidden, "ACCESS_DENIED", "Access denied to this published map", nil, nil)
	case errors.Is(err, models.ErrInvalidPublishRequest):
		models.WriteError(w, http.StatusBadRequest, "INVALID_PUBLISH_REQUEST", err.Error(), nil, nil)
	default:
		models.WriteError(w, http.StatusInternalServerError, "PUBLISH_ERROR", err.Error(), nil, nil)
	}
}

// Publish handles POST /api/publish and POST /api/maps/{workspaceId}/publish.
func (h *Handler) Publish(w http.ResponseWriter, r *http.Request, user *models.User, workspaceID string) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is supported", nil, nil)
		return
	}
	if user == nil {
		models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
		return
	}

	var req models.PublishMapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", err.Error(), nil)
		return
	}
	if req.WorkspaceID == "" && workspaceID != "" {
		req.WorkspaceID = workspaceID
	}

	sessionID := models.GetSessionID(r)
	pm, err := h.svc.PublishMap(r.Context(), sessionID, user.ID, req)
	if err != nil {
		writePublishErr(w, err)
		return
	}

	origin := requestOrigin(r)
	lanOrigin := lanRequestOrigin(r)
	viewerBase := h.appPublicURL
	if viewerBase == "" {
		viewerBase = origin
	}
	tokenSuffix := ""
	if !pm.IsPublic && pm.AccessToken != "" {
		tokenSuffix = "?token=" + pm.AccessToken
	}

	models.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"published_map":      pm,
		"map_id":             pm.MapID,
		"mode":               pm.Mode,
		"is_public":          pm.IsPublic,
		"access_token":       pm.AccessToken,
		"lan_origin":         lanOrigin,
		"composite_tile_url": fmt.Sprintf("%s/tiles/%s/composite/{0}/{1}/{2}.png%s", lanOrigin, pm.MapID, tokenSuffix),
		"map_url":            viewerBase + pm.MapURL + tokenSuffix,
		"api_map_url":        origin + pm.MapURL + tokenSuffix,
		"style_url":          origin + pm.StyleURL + tokenSuffix,
		"raster_layers":      pm.RasterLayers,
		"vector_layers":      pm.VectorLayers,
	})
}

// ListPublished handles GET /api/publish and GET /api/maps/{workspaceId}/published.
func (h *Handler) ListPublished(w http.ResponseWriter, r *http.Request, user *models.User, workspaceID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is supported", nil, nil)
		return
	}
	if user == nil {
		models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
		return
	}
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(r.URL.Query().Get("workspace_id"))
	}
	list := h.svc.ListPublishedMaps(user.ID, workspaceID)
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"published_maps": list,
		"lan_origin":     lanRequestOrigin(r),
		"total":          len(list),
	})
}

// Revoke handles DELETE /api/publish/{mapId} and POST /api/publish/{mapId}/revoke.
func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request, user *models.User, mapID string) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only DELETE or POST method is supported", nil, nil)
		return
	}
	if user == nil {
		models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
		return
	}
	if err := h.svc.RevokePublishedMap(mapID, user.ID); err != nil {
		writePublishErr(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"revoked": true,
		"map_id":  mapID,
	})
}

// GetPublishedMap handles GET /maps/{map_id} and GET /api/published-maps/{map_id}.
// Serves JSON configuration for API requests or a lightweight self-contained MapLibre HTML viewer for direct browser visits.
func (h *Handler) GetPublishedMap(w http.ResponseWriter, r *http.Request, user *models.User, mapID string, forceJSON bool) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is supported", nil, nil)
		return
	}

	userID := ""
	if user != nil {
		userID = user.ID
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	pm, err := h.svc.GetPublishedMap(mapID, userID, token)
	if err != nil {
		writePublishErr(w, err)
		return
	}

	origin := requestOrigin(r)
	tokenSuffix := ""
	if !pm.IsPublic && token != "" {
		tokenSuffix = "?token=" + token
	}

	accept := strings.ToLower(r.Header.Get("Accept"))
	if !forceJSON && strings.Contains(accept, "text/html") && !strings.Contains(accept, "application/json") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if pm.IsPublic {
			w.Header().Set("Cache-Control", "public, max-age=60")
		} else {
			w.Header().Set("Cache-Control", "private, no-store")
		}
		_, _ = w.Write([]byte(renderLightweightPublishedViewerHTML(pm, origin+pm.StyleURL+tokenSuffix)))
		return
	}

	// Build sanitized public payload (never exposes AccessToken to non-owners or internal TileDir)
	sanitized := *pm
	if userID == "" || userID != pm.OwnerID {
		sanitized.AccessToken = ""
	}
	if pm.IsPublic {
		w.Header().Set("Cache-Control", "public, max-age=60")
	} else {
		w.Header().Set("Cache-Control", "private, no-store")
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"published_map": sanitized,
		"style_url":     origin + pm.StyleURL + tokenSuffix,
		"api_origin":    origin,
	})
}

// GetStyleJSON handles GET /maps/{map_id}/style.json.
func (h *Handler) GetStyleJSON(w http.ResponseWriter, r *http.Request, user *models.User, mapID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is supported", nil, nil)
		return
	}
	userID := ""
	if user != nil {
		userID = user.ID
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	pm, err := h.svc.GetPublishedMap(mapID, userID, token)
	if err != nil {
		writePublishErr(w, err)
		return
	}

	tokenForURLs := ""
	if !pm.IsPublic {
		tokenForURLs = token
		if tokenForURLs == "" && userID == pm.OwnerID {
			tokenForURLs = pm.AccessToken
		}
	}

	styleDoc := h.svc.BuildMapLibreStyle(pm, requestOrigin(r), tokenForURLs)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	if pm.IsPublic {
		w.Header().Set("Cache-Control", "public, max-age=300")
	} else {
		w.Header().Set("Cache-Control", "private, no-store")
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(styleDoc)
}

// GetLayerMetadata handles GET /tiles/{map_id}/{layer_type}/{layer_id}/metadata.json.
func (h *Handler) GetLayerMetadata(w http.ResponseWriter, r *http.Request, user *models.User, mapID, layerType, layerID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is supported", nil, nil)
		return
	}
	userID := ""
	if user != nil {
		userID = user.ID
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	pm, err := h.svc.GetPublishedMap(mapID, userID, token)
	if err != nil {
		writePublishErr(w, err)
		return
	}

	tokenForURLs := ""
	if !pm.IsPublic {
		tokenForURLs = token
		if tokenForURLs == "" && userID == pm.OwnerID {
			tokenForURLs = pm.AccessToken
		}
	}

	metaDoc, err := h.svc.BuildLayerTileJSON(pm, layerType, layerID, requestOrigin(r), tokenForURLs)
	if err != nil {
		writePublishErr(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	if pm.IsPublic {
		w.Header().Set("Cache-Control", "public, max-age=300")
	} else {
		w.Header().Set("Cache-Control", "private, no-store")
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(metaDoc)
}

// ServePublishedTile handles:
//   - GET /tiles/{map_id}/raster/{layer_id}/{z}/{x}/{y}.png|.jpg
//   - GET /tiles/{map_id}/vector/{layer_id}/{z}/{x}/{y}.pbf
func (h *Handler) ServePublishedTile(w http.ResponseWriter, r *http.Request, user *models.User, mapID, layerType, layerID, zStr, xStr, yRaw string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET/HEAD methods are supported", nil, nil)
		return
	}
	userID := ""
	if user != nil {
		userID = user.ID
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	pm, err := h.svc.GetPublishedMap(mapID, userID, token)
	if err != nil {
		writePublishErr(w, err)
		return
	}

	yClean := yRaw
	if dotIdx := strings.IndexByte(yClean, '.'); dotIdx >= 0 {
		yClean = yClean[:dotIdx]
	}

	// If a browser or GIS validator opens/probes the literal {z}/{x}/{y} template URL directly,
	// render the live MapLibre viewer for HTML browsers or return 200 OK metadata for probes.
	isTemplatePlaceholder := (strings.Contains(zStr, "{") || strings.Contains(strings.ToLower(zStr), "%7b")) &&
		(strings.Contains(xStr, "{") || strings.Contains(strings.ToLower(xStr), "%7b")) &&
		(strings.Contains(yClean, "{") || strings.Contains(strings.ToLower(yClean), "%7b"))
	if isTemplatePlaceholder {
		tokenSuffix := ""
		if !pm.IsPublic && token != "" {
			tokenSuffix = "?token=" + token
		}
		if w.Header().Get("Access-Control-Allow-Origin") == "" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.Contains(r.Header.Get("Accept"), "text/html") {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(renderLightweightPublishedViewerHTML(pm, requestOrigin(r)+pm.StyleURL+tokenSuffix)))
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"tilejson": "3.0.0",
			"name":     pm.Name,
			"tiles":    []string{requestOrigin(r) + r.URL.Path + tokenSuffix},
			"bounds":   pm.Bounds,
			"minzoom":  pm.MinZoom,
			"maxzoom":  pm.MaxZoom,
		})
		return
	}

	z, errZ := strconv.Atoi(zStr)
	x, errX := strconv.Atoi(xStr)
	y, errY := strconv.Atoi(yClean)
	if errZ != nil || errX != nil || errY != nil || z < 0 || z > 25 || x < 0 || y < 0 {
		models.WriteError(w, http.StatusBadRequest, "INVALID_TILE_COORDS", "Invalid z/x/y tile coordinates", nil, nil)
		return
	}

	var data []byte
	var ext string

	// Support unified multi-layer vector tile URL: /tiles/{map_id}/vector/all/{z}/{x}/{y}.pbf
	if strings.EqualFold(layerType, "vector") && strings.EqualFold(layerID, "all") {
		if len(pm.VectorLayers) == 0 {
			models.WriteError(w, http.StatusNotFound, "LAYER_NOT_FOUND", "No vector layers published in map", nil, nil)
			return
		}
		var mergedRaw bytes.Buffer
		for _, vl := range pm.VectorLayers {
			tp, _, rErr := h.svc.ResolvePublishedTilePath(pm, "vector", vl.LayerID, zStr, xStr, yClean)
			if rErr != nil || tp == "" {
				continue
			}
			raw, readErr := os.ReadFile(tp)
			if readErr != nil || len(raw) == 0 {
				continue
			}
			if len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
				if gr, gErr := gzip.NewReader(bytes.NewReader(raw)); gErr == nil {
					if dec, dErr := io.ReadAll(gr); dErr == nil && len(dec) > 0 {
						raw = dec
					}
					_ = gr.Close()
				}
			}
			mergedRaw.Write(raw)
		}
		if mergedRaw.Len() == 0 {
			if w.Header().Get("Access-Control-Allow-Origin") == "" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var gzBuf bytes.Buffer
		gw := gzip.NewWriter(&gzBuf)
		_, _ = gw.Write(mergedRaw.Bytes())
		_ = gw.Close()
		data = gzBuf.Bytes()
		ext = ".pbf"
	} else {
		var tilePath string
		var resolveErr error
		// For rotated affine rasters (4 corners), prefer dynamic affine-warped PNG rendering
		// so orientation matches the workspace pixel-for-pixel across all zoom levels.
		preferDynamicRaster := false
		if strings.EqualFold(layerType, "raster") {
			for _, rl := range pm.RasterLayers {
				if rl.LayerID == layerID && len(rl.Corners) == 4 && rl.ImagePath != "" {
					preferDynamicRaster = true
					break
				}
			}
		}
		if preferDynamicRaster {
			if dynData, dynErr := h.svc.RenderPublishedRasterTile(r.Context(), pm, layerID, z, x, y); dynErr == nil {
				if len(dynData) == 0 {
					if w.Header().Get("Access-Control-Allow-Origin") == "" {
						w.Header().Set("Access-Control-Allow-Origin", "*")
					}
					w.WriteHeader(http.StatusNoContent)
					return
				}
				data = dynData
				ext = ".png"
			}
		}

		if len(data) == 0 {
			tilePath, ext, resolveErr = h.svc.ResolvePublishedTilePath(pm, layerType, layerID, zStr, xStr, yClean)
			if errors.Is(resolveErr, models.ErrPublishedMapNotFound) {
				models.WriteError(w, http.StatusNotFound, "LAYER_NOT_FOUND", "Published layer not found in map", nil, nil)
				return
			}
			if (resolveErr != nil || tilePath == "") && strings.EqualFold(layerType, "raster") {
				// Dynamic z0-z22 fallback when a raster tile wasn't pre-rendered for this zoom level
				if dynData, dynErr := h.svc.RenderPublishedRasterTile(r.Context(), pm, layerID, z, x, y); dynErr == nil && len(dynData) > 0 {
					data = dynData
					ext = ".png"
				}
			}
		}

		if len(data) == 0 {
			if resolveErr != nil || tilePath == "" {
				if w.Header().Get("Access-Control-Allow-Origin") == "" {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			var rErr error
			data, rErr = os.ReadFile(tilePath)
			if rErr != nil || len(data) == 0 {
				if w.Header().Get("Access-Control-Allow-Origin") == "" {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
	}

	// ETag support for efficient HTTP caching
	sum := sha256.Sum256(data)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	if match := r.Header.Get("If-None-Match"); match == etag {
		if w.Header().Get("Access-Control-Allow-Origin") == "" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.WriteHeader(http.StatusNotModified)
		return
	}

	cType := "application/octet-stream"
	switch strings.ToLower(ext) {
	case ".png":
		cType = "image/png"
	case ".jpg", ".jpeg":
		cType = "image/jpeg"
	case ".pbf", ".mvt":
		cType = "application/vnd.mapbox-vector-tile"
		if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
			w.Header().Set("Content-Encoding", "gzip")
		}
	}

	w.Header().Set("Content-Type", cType)
	w.Header().Set("ETag", etag)
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	if pm.IsPublic {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	} else {
		w.Header().Set("Cache-Control", "private, max-age=300")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// ServeCompositeTile handles GET/HEAD /tiles/{map_id}/composite/{z}/{x}/{y}.png
// Renders all published raster + vector layers (polygons, lines, and points) into 256x256 PNG tiles across z0-z18+.
// Supports both Viewscape {0}/{1}/{2} and standard {z}/{x}/{y} placeholders.
func (h *Handler) ServeCompositeTile(w http.ResponseWriter, r *http.Request, user *models.User, mapID, zStr, xStr, yRaw string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET/HEAD methods are supported", nil, nil)
		return
	}
	userID := ""
	if user != nil {
		userID = user.ID
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	pm, err := h.svc.GetPublishedMap(mapID, userID, token)
	if err != nil {
		writePublishErr(w, err)
		return
	}

	yClean := yRaw
	if dotIdx := strings.IndexByte(yClean, '.'); dotIdx >= 0 {
		yClean = yClean[:dotIdx]
	}

	// Cleanly handle literal {0}/{1}/{2} or {z}/{x}/{y} template URLs opened in browser or probed by Viewscape
	isTemplate := (strings.Contains(zStr, "{") || strings.Contains(strings.ToLower(zStr), "%7b")) &&
		(strings.Contains(xStr, "{") || strings.Contains(strings.ToLower(xStr), "%7b")) &&
		(strings.Contains(yClean, "{") || strings.Contains(strings.ToLower(yClean), "%7b"))
	if isTemplate {
		tokenSuffix := ""
		if !pm.IsPublic && token != "" {
			tokenSuffix = "?token=" + token
		}
		if w.Header().Get("Access-Control-Allow-Origin") == "" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.Contains(r.Header.Get("Accept"), "text/html") {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(renderLightweightPublishedViewerHTML(pm, requestOrigin(r)+pm.StyleURL+tokenSuffix)))
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"tilejson": "3.0.0",
			"name":     pm.Name,
			"format":   "png",
			"tiles":    []string{fmt.Sprintf("%s/tiles/%s/composite/{0}/{1}/{2}.png%s", lanRequestOrigin(r), pm.MapID, tokenSuffix)},
			"bounds":   pm.Bounds,
			"minzoom":  0,
			"maxzoom":  18,
		})
		return
	}

	z, errZ := strconv.Atoi(zStr)
	x, errX := strconv.Atoi(xStr)
	y, errY := strconv.Atoi(yClean)
	if errZ != nil || errX != nil || errY != nil || z < 0 || z > 22 || x < 0 || y < 0 {
		models.WriteError(w, http.StatusBadRequest, "INVALID_TILE_COORDS", "Invalid z/x/y tile coordinates", nil, nil)
		return
	}

	data, rErr := h.svc.RenderCompositePNGTile(r.Context(), pm, z, x, y)
	if rErr != nil {
		models.WriteError(w, http.StatusInternalServerError, "COMPOSITE_TILE_ERROR", "Failed rendering composite PNG tile", nil, nil)
		return
	}

	sum := sha256.Sum256(data)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	if match := r.Header.Get("If-None-Match"); match == etag {
		if w.Header().Get("Access-Control-Allow-Origin") == "" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("ETag", etag)
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	if pm.IsPublic {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	} else {
		w.Header().Set("Cache-Control", "private, max-age=300")
	}
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

// lanRequestOrigin returns a LAN-reachable http://<lan-ip>:<port> origin when r.Host is localhost/127.0.0.1
// (or respects PUBLIC_BACKEND_URL when configured), so Custom Web Tile URLs work from other devices on the same network.
func lanRequestOrigin(r *http.Request) string {
	if pub := strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_BACKEND_URL")), "/"); pub != "" {
		return pub
	}
	origin := requestOrigin(r)
	host := r.Host
	if host == "" {
		return origin
	}
	hostname, port, splitErr := net.SplitHostPort(host)
	if splitErr != nil {
		hostname = host
		port = ""
	}
	if hostname != "127.0.0.1" && hostname != "localhost" && hostname != "::1" {
		return origin
	}
	// Detect active outbound non-loopback IPv4 LAN address
	lanIP := ""
	if conn, err := net.Dial("udp", "8.8.8.8:80"); err == nil {
		if localAddr, ok := conn.LocalAddr().(*net.UDPAddr); ok && localAddr.IP != nil && !localAddr.IP.IsLoopback() {
			if ip4 := localAddr.IP.To4(); ip4 != nil {
				lanIP = ip4.String()
			}
		}
		_ = conn.Close()
	}
	if lanIP == "" {
		if ifaces, err := net.Interfaces(); err == nil {
			for _, iface := range ifaces {
				if (iface.Flags&net.FlagUp) == 0 || (iface.Flags&net.FlagLoopback) != 0 {
					continue
				}
				addrs, _ := iface.Addrs()
				for _, addr := range addrs {
					if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP != nil && !ipNet.IP.IsLoopback() {
						if ip4 := ipNet.IP.To4(); ip4 != nil {
							lanIP = ip4.String()
							break
						}
					}
				}
				if lanIP != "" {
					break
				}
			}
		}
	}
	if lanIP == "" {
		return origin
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	if port != "" {
		return fmt.Sprintf("%s://%s:%s", scheme, lanIP, port)
	}
	return fmt.Sprintf("%s://%s", scheme, lanIP)
}

// DownloadPublishedPBF handles:
//   - GET /maps/{map_id}/download/pbf      -> downloads a ZIP containing all merged {z}/{x}/{y}.pbf tiles + metadata.json + style.json
//   - GET /maps/{map_id}/download/tile.pbf -> downloads the merged root 0/0/0.pbf file directly
func (h *Handler) DownloadPublishedPBF(w http.ResponseWriter, r *http.Request, user *models.User, mapID string, singleFile bool) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is supported", nil, nil)
		return
	}
	userID := ""
	if user != nil {
		userID = user.ID
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	pm, err := h.svc.GetPublishedMap(mapID, userID, token)
	if err != nil {
		writePublishErr(w, err)
		return
	}
	if len(pm.VectorLayers) == 0 {
		models.WriteError(w, http.StatusNotFound, "NO_VECTOR_LAYERS", "No vector .pbf tiles exist in this published map", nil, nil)
		return
	}

	mergeFiles := func(paths []string) []byte {
		var merged bytes.Buffer
		for _, p := range paths {
			raw, rErr := os.ReadFile(p)
			if rErr != nil || len(raw) == 0 {
				continue
			}
			if len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
				if gr, gErr := gzip.NewReader(bytes.NewReader(raw)); gErr == nil {
					if dec, dErr := io.ReadAll(gr); dErr == nil && len(dec) > 0 {
						raw = dec
					}
					_ = gr.Close()
				}
			}
			merged.Write(raw)
		}
		return merged.Bytes()
	}

	safeName := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, strings.TrimSpace(pm.Name))
	if safeName == "" {
		safeName = pm.MapID
	}

	tileFiles := h.svc.CollectPublishedVectorTileFiles(pm)
	if len(tileFiles) == 0 {
		models.WriteError(w, http.StatusNotFound, "NO_PBF_TILES", "No .pbf vector tiles found on disk for this map", nil, nil)
		return
	}

	if singleFile {
		// Prefer 0/0/0.pbf (or the lowest-zoom available tile)
		selectedKey := "0/0/0.pbf"
		if _, ok := tileFiles[selectedKey]; !ok {
			keys := make([]string, 0, len(tileFiles))
			for k := range tileFiles {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			selectedKey = keys[0]
		}
		rawPBF := mergeFiles(tileFiles[selectedKey])
		if len(rawPBF) == 0 {
			models.WriteError(w, http.StatusNotFound, "NO_PBF_TILES", "Merged .pbf tile is empty", nil, nil)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.mapbox-vector-tile")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.pbf"`, safeName))
		if w.Header().Get("Access-Control-Allow-Origin") == "" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(rawPBF)
		return
	}

	// Build ZIP archive with all merged {z}/{x}/{y}.pbf tiles + metadata.json + style.json
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)

	keys := make([]string, 0, len(tileFiles))
	for k := range tileFiles {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, relKey := range keys {
		rawPBF := mergeFiles(tileFiles[relKey])
		if len(rawPBF) == 0 {
			continue
		}
		fw, cErr := zw.Create(relKey)
		if cErr == nil {
			_, _ = fw.Write(rawPBF)
		}
	}

	tokenForURLs := ""
	if !pm.IsPublic {
		tokenForURLs = token
		if tokenForURLs == "" && userID == pm.OwnerID {
			tokenForURLs = pm.AccessToken
		}
	}
	styleDoc := h.svc.BuildMapLibreStyle(pm, requestOrigin(r), tokenForURLs)
	if sb, mErr := json.MarshalIndent(styleDoc, "", "  "); mErr == nil {
		if fw, cErr := zw.Create("style.json"); cErr == nil {
			_, _ = fw.Write(sb)
		}
	}

	metaDoc := map[string]interface{}{
		"name":        pm.Name,
		"map_id":      pm.MapID,
		"format":      "pbf",
		"minzoom":     pm.MinZoom,
		"maxzoom":     pm.MaxZoom,
		"bounds":      pm.Bounds,
		"center":      pm.Center,
		"tile_count":  len(keys),
		"tile_url":    requestOrigin(r) + "/tiles/" + pm.MapID + "/vector/all/{z}/{x}/{y}.pbf",
		"compression": "none",
	}
	if mb, mErr := json.MarshalIndent(metaDoc, "", "  "); mErr == nil {
		if fw, cErr := zw.Create("metadata.json"); cErr == nil {
			_, _ = fw.Write(mb)
		}
	}
	_ = zw.Close()

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s_pbf_tiles.zip"`, safeName))
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(zipBuf.Bytes())
}

// ServeStaticMapLibre serves local MapLibre GL assets from frontend/node_modules/maplibre-gl/dist/
// so published map viewers work across LAN devices with zero external CDN dependency.
func (h *Handler) ServeStaticMapLibre(w http.ResponseWriter, r *http.Request, filename string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var contentType string
	switch filename {
	case "maplibre-gl.css":
		contentType = "text/css; charset=utf-8"
	case "maplibre-gl.mjs", "maplibre-gl-shared.mjs", "maplibre-gl-worker.mjs":
		contentType = "application/javascript; charset=utf-8"
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}

	candidates := []string{
		"../frontend/node_modules/maplibre-gl/dist/" + filename,
		"frontend/node_modules/maplibre-gl/dist/" + filename,
		"../../frontend/node_modules/maplibre-gl/dist/" + filename,
	}
	for _, p := range candidates {
		if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
			w.Header().Set("Content-Type", contentType)
			w.Header().Set("Cache-Control", "public, max-age=86400")
			if w.Header().Get("Access-Control-Allow-Origin") == "" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
			w.WriteHeader(http.StatusOK)
			if r.Method != http.MethodHead {
				_, _ = w.Write(data)
			}
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

func renderLightweightPublishedViewerHTML(pm *models.PublishedMap, styleURL string) string {
	title := html.EscapeString(pm.Name)
	safeStyleURL := html.EscapeString(styleURL)
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>%s — Veracity Maps</title>
  <link href="/static/maplibre/maplibre-gl.css" rel="stylesheet" />
  <style>
    body { margin:0; padding:0; font-family: system-ui, sans-serif; background:#141414; color:#f4f4f5; }
    #map { position:absolute; inset:0; }
    #panel { position:absolute; top:12px; left:12px; z-index:10; width:260px; max-height:80vh; overflow:auto;
      background:rgba(20,20,22,0.92); border:1px solid #2e2e32; border-radius:10px; padding:12px; font-size:12px; }
    h3 { margin:0 0 8px 0; font-size:13px; color:#3ecf8e; }
    label { display:flex; align-items:center; gap:6px; margin:4px 0; cursor:pointer; }
  </style>
</head>
<body>
  <div id="map"></div>
  <div id="panel">
    <h3>%s</h3>
    <div id="controls">Loading layers…</div>
  </div>
  <script type="module">
    import * as maplibregl from "/static/maplibre/maplibre-gl.mjs";
    maplibregl.setWorkerUrl("/static/maplibre/maplibre-gl-worker.mjs");
    const styleUrl = "%s";
    const map = new maplibregl.Map({ container: "map", style: styleUrl });
    map.addControl(new maplibregl.NavigationControl(), "top-right");
    map.on("load", () => {
      const style = map.getStyle();
      const ctrl = document.getElementById("controls");
      ctrl.innerHTML = "";
      (style.layers || []).forEach((l) => {
        if (l.id === "osm-base-layer") return;
        const lbl = document.createElement("label");
        const cb = document.createElement("input");
        cb.type = "checkbox";
        cb.checked = (l.layout && l.layout.visibility) !== "none";
        cb.onchange = () => map.setLayoutProperty(l.id, "visibility", cb.checked ? "visible" : "none");
        lbl.appendChild(cb);
        lbl.appendChild(document.createTextNode(l.id));
        ctrl.appendChild(lbl);
      });
      map.on("click", (e) => {
        const vecLayers = (map.getStyle().layers || []).filter(l => l.id.startsWith("pub-vec-")).map(l => l.id);
        const hits = map.queryRenderedFeatures(e.point, { layers: vecLayers });
        if (!hits.length) return;
        const props = hits[0].properties || {};
        const rows = Object.entries(props).filter(([k]) => !k.startsWith("_")).map(([k,v]) => "<b>"+k+"</b>: "+v).join("<br/>");
        new maplibregl.Popup().setLngLat(e.lngLat).setHTML("<div style='color:#111;font-size:12px'>" + (rows || "Feature") + "</div>").addTo(map);
      });
    });
  </script>
</body>
</html>`, title, title, safeStyleURL)
}
