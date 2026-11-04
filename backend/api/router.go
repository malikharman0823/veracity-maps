package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/malikharman0823/veracity-maps/functions/auth"
	"github.com/malikharman0823/veracity-maps/functions/datasets"
	"github.com/malikharman0823/veracity-maps/functions/export"
	"github.com/malikharman0823/veracity-maps/functions/georeference"
	"github.com/malikharman0823/veracity-maps/functions/health"
	"github.com/malikharman0823/veracity-maps/functions/projects"
	"github.com/malikharman0823/veracity-maps/functions/upload"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
)

type Router struct {
	repo       *repository.SessionRepository
	authRepo   *repository.AuthRepository
	projRepo   *repository.ProjectRepository
	middleware *Middleware
	uploadH    *upload.Handler
	datasetsH  *datasets.Handler
	georefH    *georeference.Handler
	exportH    *export.Handler
	authH      *auth.Handler
	projectsH  *projects.Handler
}

func NewRouter(
	repo *repository.SessionRepository,
	authRepo *repository.AuthRepository,
	projRepo *repository.ProjectRepository,
	mw *Middleware,
	uh *upload.Handler,
	dh *datasets.Handler,
	gh *georeference.Handler,
	eh *export.Handler,
	ah *auth.Handler,
	ph *projects.Handler,
) *Router {
	return &Router{
		repo:       repo,
		authRepo:   authRepo,
		projRepo:   projRepo,
		middleware: mw,
		uploadH:    uh,
		datasetsH:  dh,
		georefH:    gh,
		exportH:    eh,
		authH:      ah,
		projectsH:  ph,
	}
}

func (rt *Router) BuildHandler() http.Handler {
	mux := http.HandlerFunc(rt.route)
	return rt.middleware.CORS(rt.middleware.Recovery(rt.middleware.Session(mux)))
}

func (rt *Router) route(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	if path == "" {
		path = "/"
	}

	// 1. Health
	if path == "/health" || path == "/api/health" {
		health.HealthHandler(w, r)
		return
	}

	// 1a. Authentication
	if path == "/api/auth/register" || path == "/auth/register" {
		rt.authH.Register(w, r)
		return
	}
	if path == "/api/auth/login" || path == "/auth/login" {
		rt.authH.Login(w, r)
		return
	}
	if path == "/api/auth/logout" || path == "/auth/logout" {
		rt.authH.Logout(w, r, GetToken(r))
		return
	}
	if path == "/api/auth/me" || path == "/auth/me" {
		user := GetUser(r)
		if user == nil {
			models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
			return
		}
		rt.authH.Me(w, r, user)
		return
	}

	// 1b. User Projects (Dashboard & Ownership)
	if path == "/api/projects" || path == "/projects" {
		user := GetUser(r)
		if user == nil {
			models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
			return
		}
		if r.Method == http.MethodGet {
			rt.projectsH.List(w, r, user.ID)
			return
		}
		if r.Method == http.MethodPost {
			rt.projectsH.Create(w, r, user.ID)
			return
		}
	}

	if strings.HasPrefix(path, "/api/projects/") || strings.HasPrefix(path, "/projects/") {
		user := GetUser(r)
		if user == nil {
			models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
			return
		}
		var remainder string
		if strings.HasPrefix(path, "/api/projects/") {
			remainder = strings.TrimPrefix(path, "/api/projects/")
		} else {
			remainder = strings.TrimPrefix(path, "/projects/")
		}
		parts := strings.Split(remainder, "/")
		if len(parts) == 1 {
			projectID := parts[0]
			if r.Method == http.MethodGet {
				rt.projectsH.Get(w, r, projectID, user.ID)
				return
			}
			if r.Method == http.MethodPatch || r.Method == http.MethodPut {
				rt.projectsH.Update(w, r, projectID, user.ID)
				return
			}
			if r.Method == http.MethodDelete {
				rt.projectsH.Delete(w, r, projectID, user.ID)
				return
			}
		}
	}

	// 2. Sessions
	if path == "/sessions" {
		if r.Method == http.MethodPost {
			sess := rt.repo.CreateSession()
			w.Header().Set("X-Session-ID", sess.ID)
			models.WriteJSON(w, http.StatusCreated, map[string]interface{}{
				"session_id": sess.ID,
				"created_at": sess.CreatedAt,
			})
			return
		}
	}

	if path == "/sessions/current" {
		sid := GetSessionID(r)
		if sid == "" {
			models.WriteError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "No active session ID provided", nil, nil)
			return
		}
		sess, err := rt.repo.GetSession(sid)
		if err != nil {
			models.WriteError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Session not found", nil, nil)
			return
		}
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"session_id":     sess.ID,
			"created_at":     sess.CreatedAt,
			"last_active_at": sess.LastActiveAt,
		})
		return
	}

	if path == "/sessions/current/end" || (path == "/sessions/current" && r.Method == http.MethodDelete) {
		sid := GetSessionID(r)
		if sid != "" {
			rt.repo.DeleteSession(sid)
		}
		models.WriteJSON(w, http.StatusOK, map[string]bool{"ended": true})
		return
	}

	if strings.HasPrefix(path, "/sessions/") {
		remainder := strings.TrimPrefix(path, "/sessions/")
		parts := strings.Split(remainder, "/")

		if len(parts) == 1 {
			sid := parts[0]
			if r.Method == http.MethodGet {
				sess, err := rt.repo.GetSession(sid)
				if err != nil {
					models.WriteError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Session not found", nil, nil)
					return
				}
				models.WriteJSON(w, http.StatusOK, map[string]interface{}{
					"session_id":     sess.ID,
					"created_at":     sess.CreatedAt,
					"last_active_at": sess.LastActiveAt,
				})
				return
			}
			if r.Method == http.MethodDelete {
				rt.repo.DeleteSession(sid)
				models.WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
				return
			}
		}

		if len(parts) == 2 && parts[1] == "end" && r.Method == http.MethodPost {
			rt.repo.DeleteSession(parts[0])
			models.WriteJSON(w, http.StatusOK, map[string]bool{"ended": true})
			return
		}

		if len(parts) == 2 && parts[1] == "map-config" {
			sid := parts[0]
			sess, err := rt.repo.GetSession(sid)
			if err != nil {
				models.WriteError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Session not found", nil, nil)
				return
			}
			if r.Method == http.MethodGet {
				models.WriteJSON(w, http.StatusOK, sess.MapConfig)
				return
			}
			if r.Method == http.MethodPut {
				var configData map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&configData)
				sess.MapConfig = configData
				models.WriteJSON(w, http.StatusOK, configData)
				return
			}
		}

		if len(parts) >= 2 && parts[1] == "configs" {
			sid := parts[0]
			if len(parts) == 2 {
				if r.Method == http.MethodGet {
					cfgs, err := rt.repo.ListSavedConfigs(sid)
					if err != nil {
						models.WriteError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Session not found", nil, nil)
						return
					}
					models.WriteJSON(w, http.StatusOK, cfgs)
					return
				}
				if r.Method == http.MethodPost {
					var cfg models.SavedConfig
					if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
						models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid config JSON", nil, nil)
						return
					}
					saved, err := rt.repo.SaveConfig(sid, &cfg)
					if err != nil {
						models.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil, nil)
						return
					}
					models.WriteJSON(w, http.StatusCreated, saved)
					return
				}
			}
			if len(parts) == 3 {
				configID := parts[2]
				if r.Method == http.MethodGet {
					cfg, err := rt.repo.GetSavedConfig(sid, configID)
					if err != nil {
						models.WriteError(w, http.StatusNotFound, "CONFIG_NOT_FOUND", "Config not found", nil, nil)
						return
					}
					models.WriteJSON(w, http.StatusOK, cfg)
					return
				}
				if r.Method == http.MethodDelete {
					err := rt.repo.DeleteSavedConfig(sid, configID)
					if err != nil {
						models.WriteError(w, http.StatusNotFound, "CONFIG_NOT_FOUND", "Config not found", nil, nil)
						return
					}
					models.WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
					return
				}
			}
		}
	}

	// 3. Uploads & Project Reopen
	if path == "/upload" || path == "/datasets/import-zip" || path == "/api/datasets/import" {
		rt.uploadH.UploadFile(w, r)
		return
	}
	if path == "/map-projects/inspect" || path == "/api/map-projects/inspect" {
		rt.uploadH.InspectProjectPackage(w, r)
		return
	}
	if path == "/map-projects/import" || path == "/map-projects/reopen" || path == "/map-projects/import-folder" || path == "/api/map-projects/import" || path == "/api/map-projects/reopen" {
		rt.uploadH.ReopenProject(w, r)
		return
	}
	if path == "/datasets/inspect-archive" || path == "/api/datasets/inspect-archive" {
		rt.uploadH.InspectArchive(w, r)
		return
	}
	if path == "/datasets/import-archive-selected" || path == "/api/datasets/import-archive-selected" {
		rt.uploadH.ImportArchiveSelected(w, r)
		return
	}
	if path == "/datasets/import-pdf-pages" || path == "/api/datasets/import-pdf-pages" {
		rt.uploadH.ImportPdfPages(w, r)
		return
	}

	// 3b. Document thumbnails (PDF)
	if strings.HasPrefix(path, "/documents/") && strings.Contains(path, "/pdf-thumbnail/") {
		trimmed := strings.TrimPrefix(path, "/documents/")
		parts := strings.Split(trimmed, "/")
		if len(parts) >= 3 && parts[1] == "pdf-thumbnail" {
			rt.uploadH.GetPdfThumbnail(w, r, parts[0], parts[2])
			return
		}
	}

	// 4. Datasets collection
	if path == "/datasets" || path == "/api/datasets" {
		rt.datasetsH.List(w, r)
		return
	}

	// 5. Control points direct routes
	if strings.HasPrefix(path, "/control-points/") {
		pointID := strings.TrimPrefix(path, "/control-points/")
		if r.Method == http.MethodPut {
			rt.georefH.UpdateControlPoint(w, r, pointID)
			return
		}
		if r.Method == http.MethodDelete {
			rt.georefH.DeleteControlPoint(w, r, pointID)
			return
		}
	}

	// 5b. Icon points direct routes
	if strings.HasPrefix(path, "/icon-points/") {
		pointID := strings.TrimPrefix(path, "/icon-points/")
		if r.Method == http.MethodGet {
			rt.datasetsH.GetIconPoint(w, r, pointID)
			return
		}
		if r.Method == http.MethodPut || r.Method == http.MethodPatch {
			rt.datasetsH.UpdateIconPoint(w, r, pointID)
			return
		}
		if r.Method == http.MethodDelete {
			rt.datasetsH.DeleteIconPoint(w, r, pointID)
			return
		}
	}

	// 6. Dataset sub-resources (/datasets/{id}/...)
	cleanDsPath := path
	if strings.HasPrefix(cleanDsPath, "/api/datasets/") {
		cleanDsPath = strings.TrimPrefix(cleanDsPath, "/api")
	}

	if strings.HasPrefix(cleanDsPath, "/datasets/") {
		trimmed := strings.TrimPrefix(cleanDsPath, "/datasets/")
		parts := strings.Split(trimmed, "/")

		datasetID := parts[0]

		if len(parts) == 1 {
			if r.Method == http.MethodGet {
				rt.datasetsH.Get(w, r, datasetID)
				return
			}
			if r.Method == http.MethodDelete {
				rt.datasetsH.Delete(w, r, datasetID)
				return
			}
		}

		if len(parts) == 2 {
			sub := parts[1]
			switch sub {
			case "source-image":
				rt.datasetsH.SourceImage(w, r, datasetID)
				return
			case "layers":
				rt.datasetsH.ListLayers(w, r, datasetID)
				return
			case "control-points":
				if r.Method == http.MethodGet {
					rt.georefH.ListControlPoints(w, r, datasetID)
					return
				}
				if r.Method == http.MethodPost {
					rt.georefH.CreateControlPoint(w, r, datasetID)
					return
				}
			case "icon-points":
				if r.Method == http.MethodGet {
					rt.datasetsH.ListIconPoints(w, r, datasetID)
					return
				}
				if r.Method == http.MethodPost {
					rt.datasetsH.CreateIconPoint(w, r, datasetID)
					return
				}
			case "workspace":
				rt.datasetsH.GetWorkspace(w, r, datasetID)
				return
			case "project":
				if r.Method == http.MethodGet {
					proj, err := rt.repo.GetMapProject(GetSessionID(r), datasetID)
					if err != nil || proj == nil {
						models.WriteJSON(w, http.StatusOK, map[string]interface{}{})
						return
					}
					models.WriteJSON(w, http.StatusOK, proj)
					return
				}
				if r.Method == http.MethodPut {
					var req struct {
						Name          string                 `json:"name"`
						Configuration map[string]interface{} `json:"configuration"`
					}
					_ = json.NewDecoder(r.Body).Decode(&req)
					proj := &models.MapProject{
						DatasetID:     datasetID,
						Name:          req.Name,
						Configuration: req.Configuration,
					}
					_ = rt.repo.SaveMapProject(GetSessionID(r), proj)
					models.WriteJSON(w, http.StatusOK, proj)
					return
				}
			case "background-removal":
				if r.Method == http.MethodGet {
					rt.datasetsH.GetBackgroundRemoval(w, r, datasetID)
					return
				}
				if r.Method == http.MethodPost {
					rt.datasetsH.HandleBackgroundRemoval(w, r, datasetID)
					return
				}
			case "transform":
				if r.Method == http.MethodGet {
					rt.datasetsH.GetRasterTransform(w, r, datasetID)
					return
				}
				if r.Method == http.MethodPost {
					rt.datasetsH.HandleRasterTransform(w, r, datasetID)
					return
				}
			case "georeference":
				rt.georefH.Georeference(w, r, datasetID)
				return
			case "export":
				rt.exportH.Export(w, r, datasetID)
				return
			case "inspection":
				rt.datasetsH.GetInspection(w, r, datasetID)
				return
			case "enhance":
				rt.datasetsH.HandleEnhancement(w, r, datasetID)
				return
			case "ocr":
				rt.datasetsH.HandleOCR(w, r, datasetID)
				return
			case "vectors":
				rt.datasetsH.GetVectors(w, r, datasetID)
				return
			case "mbtiles":
				rt.datasetsH.HandleGenerateTiles(w, r, datasetID)
				return
			case "annotations":
				rt.datasetsH.HandleAnnotations(w, r, datasetID)
				return
			}
		}

		if len(parts) == 3 && parts[1] == "annotations" {
			rt.datasetsH.HandleAnnotationItem(w, r, datasetID, parts[2])
			return
		}

		if len(parts) == 5 && (parts[1] == "tiles" || parts[1] == "mbtiles") {
			rt.datasetsH.ServeTile(w, r, datasetID, parts[2], parts[3], parts[4])
			return
		}

		if len(parts) >= 3 && parts[1] == "workspace" {
			sub := parts[2]
			switch sub {
			case "records":
				rt.datasetsH.GetWorkspaceRecords(w, r, datasetID)
				return
			case "state":
				rt.datasetsH.PatchWorkspaceState(w, r, datasetID)
				return
			case "members":
				if len(parts) == 3 && r.Method == http.MethodPost {
					rt.datasetsH.AttachWorkspaceMember(w, r, datasetID)
					return
				}
				if len(parts) == 4 {
					memberID := parts[3]
					if r.Method == http.MethodPatch || r.Method == http.MethodPut {
						rt.datasetsH.PatchWorkspaceMember(w, r, datasetID, memberID)
						return
					}
					if r.Method == http.MethodDelete {
						rt.datasetsH.DetachWorkspaceMember(w, r, datasetID, memberID)
						return
					}
				}
			}
		}

		if len(parts) == 3 && parts[1] == "layers" {
			layerID := parts[2]
			rt.datasetsH.UpdateLayer(w, r, datasetID, layerID)
			return
		}
	}

	// 6a-2. Standalone Viewscape tile serving endpoints (/viewscape/{project_id}/tiles/{z}/{x}/{y})
	if strings.HasPrefix(path, "/viewscape/") || strings.HasPrefix(path, "/api/viewscape/") {
		vPath := strings.TrimPrefix(path, "/api")
		vPath = strings.TrimPrefix(vPath, "/viewscape/")
		vParts := strings.Split(vPath, "/")
		if len(vParts) == 5 && (vParts[1] == "tiles" || vParts[1] == "mbtiles") {
			rt.datasetsH.ServeTile(w, r, vParts[0], vParts[2], vParts[3], vParts[4])
			return
		}
	}

	// 6b. Buildings & Floors collection and resources
	if path == "/buildings" || path == "/api/buildings" {
		if r.Method == http.MethodGet {
			rt.datasetsH.ListBuildings(w, r)
			return
		}
		if r.Method == http.MethodPost {
			rt.datasetsH.CreateBuilding(w, r)
			return
		}
	}

	cleanBldPath := path
	if strings.HasPrefix(cleanBldPath, "/api/buildings/") {
		cleanBldPath = strings.TrimPrefix(cleanBldPath, "/api")
	}
	if strings.HasPrefix(cleanBldPath, "/buildings/") {
		trimmed := strings.TrimPrefix(cleanBldPath, "/buildings/")
		parts := strings.Split(trimmed, "/")
		buildingID := parts[0]

		if len(parts) == 1 {
			if r.Method == http.MethodGet {
				rt.datasetsH.GetBuilding(w, r, buildingID)
				return
			}
			if r.Method == http.MethodPut || r.Method == http.MethodPatch {
				rt.datasetsH.UpdateBuilding(w, r, buildingID)
				return
			}
			if r.Method == http.MethodDelete {
				rt.datasetsH.DeleteBuilding(w, r, buildingID)
				return
			}
		}

		if len(parts) == 2 && parts[1] == "propagate" {
			if r.Method == http.MethodPost {
				rt.datasetsH.PropagateBuildingGeoreferenceHandler(w, r, buildingID)
				return
			}
		}

		if len(parts) >= 2 && parts[1] == "floors" {
			if len(parts) == 2 {
				if r.Method == http.MethodPost {
					rt.datasetsH.AddBuildingFloor(w, r, buildingID)
					return
				}
			}
			if len(parts) == 3 {
				floorID := parts[2]
				if r.Method == http.MethodPut || r.Method == http.MethodPatch {
					rt.datasetsH.UpdateBuildingFloor(w, r, buildingID, floorID)
					return
				}
				if r.Method == http.MethodDelete {
					rt.datasetsH.DeleteBuildingFloor(w, r, buildingID, floorID)
					return
				}
			}
		}
	}

	// 7. Icons
	if path == "/icons" || path == "/api/icons" {
		rt.datasetsH.ListIcons(w, r)
		return
	}
	if strings.HasPrefix(path, "/icons/") {
		iconName := strings.TrimPrefix(path, "/icons/")
		rt.datasetsH.GetIcon(w, r, iconName)
		return
	}
	if strings.HasPrefix(path, "/api/icons/") {
		iconName := strings.TrimPrefix(path, "/api/icons/")
		rt.datasetsH.GetIcon(w, r, iconName)
		return
	}

	// 8. Export download
	if strings.HasPrefix(path, "/api/exports/") && strings.HasSuffix(path, "/download") {
		trimmed := strings.TrimPrefix(path, "/api/exports/")
		trimmed = strings.TrimSuffix(trimmed, "/download")
		exportID := strings.Trim(trimmed, "/")
		rt.exportH.Download(w, r, exportID)
		return
	}

	models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Endpoint not found: "+r.URL.Path, nil, nil)
}
