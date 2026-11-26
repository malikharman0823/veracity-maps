package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/malikharman0823/veracity-maps/functions/auth"
	"github.com/malikharman0823/veracity-maps/functions/baselayers"
	"github.com/malikharman0823/veracity-maps/functions/datasets"
	"github.com/malikharman0823/veracity-maps/functions/export"
	"github.com/malikharman0823/veracity-maps/functions/georeference"
	"github.com/malikharman0823/veracity-maps/functions/gis"
	"github.com/malikharman0823/veracity-maps/functions/health"
	"github.com/malikharman0823/veracity-maps/functions/maps"
	"github.com/malikharman0823/veracity-maps/functions/processing"
	"github.com/malikharman0823/veracity-maps/functions/profile"
	"github.com/malikharman0823/veracity-maps/functions/projects"
	"github.com/malikharman0823/veracity-maps/functions/publish"
	"github.com/malikharman0823/veracity-maps/functions/upload"
	"github.com/malikharman0823/veracity-maps/functions/uploads"
	"github.com/malikharman0823/veracity-maps/functions/workspaceregistry"
	gislib "github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
	"github.com/malikharman0823/veracity-maps/storage"
)

type Router struct {
	repo            *repository.SessionRepository
	authRepo        repository.AuthRepo
	projRepo        *repository.ProjectRepository
	middleware      *Middleware
	uploadH         *upload.Handler
	unifiedUploadsH *uploads.Handler
	datasetsH       *datasets.Handler
	georefH         *georeference.Handler
	exportH         *export.Handler
	authH           *auth.Handler
	projectsH       *projects.Handler
	profileH        *profile.Handler
	mapsH           *maps.Handler
	gisH            *gis.Handler
	procH           *processing.Handler
	baseLayerH      *baselayers.Handler
	wsRegH          *workspaceregistry.Handler
	publishH        *publish.Handler
}

func (rt *Router) SetPublishHandler(ph *publish.Handler) {
	rt.publishH = ph
	if ph != nil && ph.GetService() != nil {
		if rt.gisH != nil {
			ph.GetService().SetGISService(rt.gisH.GetService())
		}
		if rt.mapsH != nil {
			ph.GetService().SetMapRepo(rt.mapsH.GetMapRepo())
		}
	}
}

func (rt *Router) SetMapsHandler(mh *maps.Handler) {
	rt.mapsH = mh
	if mh != nil && rt.gisH != nil {
		gisSvc := services.NewGISService(rt.gisH.GetService().GetGISRepo(), mh.GetMapRepo(), rt.repo)
		rt.gisH = gis.NewHandler(gisSvc)
		rt.procH = processing.NewHandler(services.NewProcessingService(repository.NewMemoryProcessingRepository(), gisSvc, mh.GetMapRepo(), rt.repo, ""))
		if rt.wsRegH != nil {
			rt.wsRegH.GetService().SetGISService(gisSvc)
			rt.wsRegH.GetService().SetMapRepo(mh.GetMapRepo())
		}
	}
	if mh != nil && rt.baseLayerH != nil {
		rt.baseLayerH.GetService().SetMapRepo(mh.GetMapRepo())
	}
}

func (rt *Router) SetBaseLayerHandler(blh *baselayers.Handler) {
	rt.baseLayerH = blh
	if blh != nil && rt.mapsH != nil {
		blh.GetService().SetMapRepo(rt.mapsH.GetMapRepo())
	}
	if blh != nil && rt.exportH != nil && rt.exportH.GetService() != nil {
		rt.exportH.GetService().SetBaseLayerService(blh.GetService())
	}
}

func (rt *Router) SetWorkspaceRegistryHandler(wrh *workspaceregistry.Handler) {
	rt.wsRegH = wrh
	if wrh != nil {
		if rt.mapsH != nil {
			wrh.GetService().SetMapRepo(rt.mapsH.GetMapRepo())
		}
		if rt.gisH != nil {
			wrh.GetService().SetGISService(rt.gisH.GetService())
		}
	}
}

func (rt *Router) SetGISHandler(gh *gis.Handler) {
	rt.gisH = gh
	if gh != nil {
		var mr repository.MapRepo
		if rt.mapsH != nil {
			mr = rt.mapsH.GetMapRepo()
		}
		rt.procH = processing.NewHandler(services.NewProcessingService(repository.NewMemoryProcessingRepository(), gh.GetService(), mr, rt.repo, ""))
		if rt.wsRegH != nil {
			rt.wsRegH.GetService().SetGISService(gh.GetService())
		}
		if rt.exportH != nil && rt.exportH.GetService() != nil {
			rt.exportH.GetService().SetGISService(gh.GetService())
		}
	}
}

func (rt *Router) SetProcessingHandler(ph *processing.Handler) {
	rt.procH = ph
}

func (rt *Router) SetUnifiedUploadsHandler(uh *uploads.Handler) {
	rt.unifiedUploadsH = uh
}

func NewRouter(
	repo *repository.SessionRepository,
	authRepo repository.AuthRepo,
	projRepo *repository.ProjectRepository,
	mw *Middleware,
	uh *upload.Handler,
	dh *datasets.Handler,
	gh *georeference.Handler,
	eh *export.Handler,
	ah *auth.Handler,
	ph *projects.Handler,
	profileHandlers ...*profile.Handler,
) *Router {
	defaultGISService := services.NewGISService(repository.NewMemoryGISRepository(), nil, repo)
	defaultProcService := services.NewProcessingService(repository.NewMemoryProcessingRepository(), defaultGISService, nil, repo, "")
	defaultBaseLayerService := services.NewBaseLayerService(repository.NewMemoryBaseLayerRepository(), repo, nil, nil)
	defaultWSRegService := services.NewWorkspaceRegistryService(repository.NewMemoryWorkspaceRegistryRepository(), defaultGISService, repo, nil)
	defaultPrivateStorage, _ := storage.NewPrivateStorage("", 0)
	defaultUnifiedImportSvc := services.NewUnifiedImportService(defaultPrivateStorage, repo, gislib.DefaultUploadLimits())
	router := &Router{
		repo:            repo,
		authRepo:        authRepo,
		projRepo:        projRepo,
		middleware:      mw,
		uploadH:         uh,
		unifiedUploadsH: uploads.NewHandler(defaultUnifiedImportSvc),
		datasetsH:       dh,
		georefH:         gh,
		exportH:         eh,
		authH:           ah,
		projectsH:       ph,
		gisH:            gis.NewHandler(defaultGISService),
		procH:           processing.NewHandler(defaultProcService),
		baseLayerH:      baselayers.NewHandler(defaultBaseLayerService),
		wsRegH:          workspaceregistry.NewHandler(defaultWSRegService),
	}
	if len(profileHandlers) > 0 {
		router.profileH = profileHandlers[0]
	}
	return router
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
	if strings.HasPrefix(path, "/api/v1/") {
		path = "/api/" + strings.TrimPrefix(path, "/api/v1/")
	} else if path == "/api/v1" {
		path = "/api"
	}

	// 1. Health
	if path == "/health" || path == "/api/health" {
		health.HealthHandler(w, r)
		return
	}

	// 1a. Authentication
	if path == "/api/auth/resend-verification" {
		user := GetUser(r)
		if user == nil {
			models.WriteError(w, 401, "UNAUTHORIZED", "Authentication required", nil, nil)
			return
		}
		rt.authH.ResendVerification(w, r, user)
		return
	}
	if path == "/api/auth/verify-email" && rt.profileH != nil {
		rt.profileH.VerifyEmail(w, r)
		return
	}
	if rt.profileH != nil && (strings.HasPrefix(path, "/api/profile") || strings.HasPrefix(path, "/profile")) {
		user := GetUser(r)
		if user == nil {
			models.WriteError(w, 401, "UNAUTHORIZED", "Authentication required", nil, nil)
			return
		}
		switch path {
		case "/api/profile", "/profile":
			rt.profileH.Profile(w, r, user.ID)
		case "/api/profile/dashboard", "/profile/dashboard":
			rt.profileH.Dashboard(w, r, user.ID)
		case "/api/profile/files", "/profile/files":
			rt.profileH.Files(w, r, user.ID)
		case "/api/profile/avatar", "/profile/avatar":
			rt.profileH.Avatar(w, r, user.ID)
		case "/api/profile/password", "/profile/password":
			rt.profileH.Password(w, r, user.ID, GetToken(r))
		case "/api/profile/email-change", "/profile/email-change":
			rt.profileH.EmailChange(w, r, user.ID)
		default:
			if strings.HasPrefix(path, "/api/profile/files/") || strings.HasPrefix(path, "/profile/files/") {
				fileID := strings.TrimPrefix(path, "/api/profile/files/")
				fileID = strings.TrimPrefix(fileID, "/profile/files/")
				rt.profileH.FileItem(w, r, user.ID, fileID)
				return
			}
			models.WriteError(w, 404, "NOT_FOUND", "Profile endpoint not found", nil, nil)
		}
		return
	}
	if path == "/api/auth/otp/send" || path == "/auth/otp/send" || path == "/api/auth/send-otp" {
		rt.authH.SendOTP(w, r)
		return
	}
	if path == "/api/auth/otp/verify" || path == "/auth/otp/verify" || path == "/api/auth/verify-otp" {
		rt.authH.VerifyOTP(w, r)
		return
	}
	if path == "/api/auth/test-login" || path == "/auth/test-login" {
		rt.authH.TestLogin(w, r)
		return
	}
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

	// 1a-unified-uploads. Unified User-Isolated Upload, Atomic Import, Job & Dataset API
	if rt.unifiedUploadsH != nil {
		if path == "/api/uploads" {
			user := GetUser(r)
			if r.Method == http.MethodPost {
				rt.unifiedUploadsH.PostUpload(w, r, user)
				return
			}
			if r.Method == http.MethodGet {
				rt.unifiedUploadsH.ListUploads(w, r, user)
				return
			}
			models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Use GET or POST for /api/uploads", nil, nil)
			return
		}
		if strings.HasPrefix(path, "/api/uploads/") {
			uploadID := strings.TrimPrefix(path, "/api/uploads/")
			if uploadID != "" && !strings.Contains(uploadID, "/") && r.Method == http.MethodGet {
				rt.unifiedUploadsH.GetUpload(w, r, GetUser(r), uploadID)
				return
			}
		}
		if strings.HasPrefix(path, "/api/imports/") {
			uploadID := strings.TrimPrefix(path, "/api/imports/")
			if uploadID != "" && !strings.Contains(uploadID, "/") && r.Method == http.MethodPost {
				rt.unifiedUploadsH.StartImport(w, r, GetUser(r), uploadID)
				return
			}
		}
		if strings.HasPrefix(path, "/api/jobs/") {
			rest := strings.TrimPrefix(path, "/api/jobs/")
			if strings.HasSuffix(rest, "/cancel") && r.Method == http.MethodPost {
				jobID := strings.TrimSuffix(rest, "/cancel")
				if jobID != "" && !strings.Contains(jobID, "/") {
					rt.unifiedUploadsH.CancelJob(w, r, GetUser(r), jobID)
					return
				}
			}
			if rest != "" && !strings.Contains(rest, "/") && r.Method == http.MethodGet {
				rt.unifiedUploadsH.GetJob(w, r, GetUser(r), rest)
				return
			}
		}
		if path == "/api/unified-datasets" && r.Method == http.MethodGet {
			rt.unifiedUploadsH.ListDatasets(w, r, GetUser(r))
			return
		}
		if strings.HasPrefix(path, "/api/unified-datasets/") {
			dsID := strings.TrimPrefix(path, "/api/unified-datasets/")
			if dsID != "" && !strings.Contains(dsID, "/") {
				if r.Method == http.MethodGet {
					rt.unifiedUploadsH.GetDataset(w, r, GetUser(r), dsID)
					return
				}
				if r.Method == http.MethodDelete {
					rt.unifiedUploadsH.DeleteDataset(w, r, GetUser(r), dsID)
					return
				}
			}
		}
	}

	// 1a-share. Public Share Link Endpoint (/api/share/:token)
	if rt.mapsH != nil && (strings.HasPrefix(path, "/api/share/") || strings.HasPrefix(path, "/share/")) {
		token := strings.TrimPrefix(path, "/api/share/")
		token = strings.TrimPrefix(token, "/share/")
		if token != "" {
			rt.mapsH.ResolveShareLink(w, r, token)
			return
		}
	}

	// 1a-publish. Combined / Raster / Vector Web Tile Publishing Endpoints
	//   POST   /api/publish
	//   GET    /api/publish
	//   DELETE /api/publish/{map_id}
	//   POST   /api/publish/{map_id}/revoke
	//   GET    /api/published-maps/{map_id}
	//   GET    /maps/{map_id}              (published map_id starting with pub_)
	//   GET    /maps/{map_id}/style.json
	//   GET    /tiles/{map_id}/{layer_type}/{layer_id}/metadata.json
	//   GET    /tiles/{map_id}/raster/{layer_id}/{z}/{x}/{y}.{png|jpg}
	//   GET    /tiles/{map_id}/vector/{layer_id}/{z}/{x}/{y}.pbf
	if rt.publishH != nil {
		if path == "/api/export/custom-local-tiles" || path == "/export/custom-local-tiles" {
			user := GetUser(r)
			sessionID := strings.TrimSpace(r.Header.Get("X-Session-ID"))
			rt.publishH.ExportCustomLocalTiles(w, r, user, sessionID)
			return
		}
		if path == "/api/publish" || path == "/publish" {
			user := GetUser(r)
			if r.Method == http.MethodPost {
				rt.publishH.Publish(w, r, user, "")
				return
			}
			if r.Method == http.MethodGet {
				rt.publishH.ListPublished(w, r, user, "")
				return
			}
		}
		if strings.HasPrefix(path, "/api/publish/") || strings.HasPrefix(path, "/publish/") {
			rem := strings.TrimPrefix(path, "/api/publish/")
			rem = strings.TrimPrefix(rem, "/publish/")
			parts := strings.Split(rem, "/")
			user := GetUser(r)
			if len(parts) == 1 && parts[0] != "" {
				rt.publishH.Revoke(w, r, user, parts[0])
				return
			}
			if len(parts) == 2 && parts[0] != "" && parts[1] == "revoke" {
				rt.publishH.Revoke(w, r, user, parts[0])
				return
			}
		}
		if strings.HasPrefix(path, "/api/published-maps/") {
			mapID := strings.TrimPrefix(path, "/api/published-maps/")
			if mapID != "" && !strings.Contains(mapID, "/") {
				rt.publishH.GetPublishedMap(w, r, GetUser(r), mapID, true)
				return
			}
		}
		if strings.HasPrefix(path, "/tiles/") || strings.HasPrefix(path, "/api/tiles/") {
			rem := strings.TrimPrefix(path, "/api/tiles/")
			rem = strings.TrimPrefix(rem, "/tiles/")
			parts := strings.Split(rem, "/")
			// /tiles/{map_id}/{layer_type}/{layer_id}/metadata.json
			if len(parts) == 4 && parts[3] == "metadata.json" {
				rt.publishH.GetLayerMetadata(w, r, GetUser(r), parts[0], parts[1], parts[2])
				return
			}
			// /tiles/{map_id}/composite/{z}/{x}/{y}.png (Viewscape-compatible composite PNG tile)
			if len(parts) == 5 && parts[1] == "composite" {
				rt.publishH.ServeCompositeTile(w, r, GetUser(r), parts[0], parts[2], parts[3], parts[4])
				return
			}
			// /tiles/{map_id}/{layer_type}/{layer_id}/{z}/{x}/{y}.{ext}
			if len(parts) == 6 && (parts[1] == "raster" || parts[1] == "vector") {
				rt.publishH.ServePublishedTile(w, r, GetUser(r), parts[0], parts[1], parts[2], parts[3], parts[4], parts[5])
				return
			}
		}
		if path == "/favicon.ico" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if strings.HasPrefix(path, "/static/maplibre/") {
			rt.publishH.ServeStaticMapLibre(w, r, strings.TrimPrefix(path, "/static/maplibre/"))
			return
		}
		if strings.HasPrefix(path, "/maps/") || strings.HasPrefix(path, "/api/maps/") {
			rem := strings.TrimPrefix(path, "/api/maps/")
			rem = strings.TrimPrefix(rem, "/maps/")
			parts := strings.Split(rem, "/")
			if len(parts) == 2 && parts[0] != "" && parts[1] == "style.json" {
				rt.publishH.GetStyleJSON(w, r, GetUser(r), parts[0])
				return
			}
			if len(parts) == 3 && parts[0] != "" && parts[1] == "download" && (parts[2] == "pbf" || parts[2] == "tile.pbf") {
				rt.publishH.DownloadPublishedPBF(w, r, GetUser(r), parts[0], parts[2] == "tile.pbf")
				return
			}
			if len(parts) == 2 && parts[0] != "" && parts[1] == "publish" && r.Method == http.MethodPost {
				rt.publishH.Publish(w, r, GetUser(r), parts[0])
				return
			}
			if len(parts) == 2 && parts[0] != "" && parts[1] == "published" && r.Method == http.MethodGet {
				rt.publishH.ListPublished(w, r, GetUser(r), parts[0])
				return
			}
			if len(parts) == 1 && strings.HasPrefix(parts[0], "pub_") && r.Method == http.MethodGet {
				rt.publishH.GetPublishedMap(w, r, GetUser(r), parts[0], strings.HasPrefix(path, "/api/"))
				return
			}
		}
	}

	// 1a-inv. Map Share Invitations Inspect/Accept/Decline (/api/map-invitations/:token[/accept|decline])
	if rt.mapsH != nil && strings.HasPrefix(path, "/api/map-invitations/") {
		remainder := strings.TrimPrefix(path, "/api/map-invitations/")
		parts := strings.Split(remainder, "/")
		if len(parts) == 1 && parts[0] != "" && r.Method == http.MethodGet {
			rt.mapsH.GetInvitation(w, r, GetUser(r), parts[0])
			return
		}
		if len(parts) >= 2 {
			token := parts[0]
			action := parts[1]
			user := GetUser(r)
			if user == nil {
				models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
				return
			}
			if action == "accept" {
				rt.mapsH.AcceptInvitation(w, r, user, token)
				return
			}
			if action == "decline" {
				rt.mapsH.DeclineInvitation(w, r, user, token)
				return
			}
		}
	}

	// 1a-files. Personal User Files (/api/user/files or /api/files)
	if rt.mapsH != nil && (path == "/api/user/files" || strings.HasPrefix(path, "/api/user/files/") || path == "/api/files" || strings.HasPrefix(path, "/api/files/")) {
		user := GetUser(r)
		if user == nil {
			models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
			return
		}
		if path == "/api/user/files" || path == "/api/files" {
			if r.Method == http.MethodGet {
				rt.mapsH.ListUserFiles(w, r, user)
				return
			}
		}
		var fileID string
		if strings.HasPrefix(path, "/api/user/files/") {
			fileID = strings.TrimPrefix(path, "/api/user/files/")
		} else {
			fileID = strings.TrimPrefix(path, "/api/files/")
		}
		if fileID != "" && r.Method == http.MethodDelete {
			rt.mapsH.DeleteUserFile(w, r, user, fileID)
			return
		}
	}

	// 1a-baselayers. Custom Base Layers & Multiple Map Views Endpoints
	if rt.baseLayerH != nil && (path == "/api/map-views" || strings.HasPrefix(path, "/api/map-views/")) {
		if path == "/api/map-views" {
			rt.baseLayerH.ListMapViews(w, r)
			return
		}
		viewType := strings.TrimPrefix(path, "/api/map-views/")
		if viewType != "" && !strings.Contains(viewType, "/") {
			rt.baseLayerH.GetMapView(w, r, viewType)
			return
		}
	}

	if rt.baseLayerH != nil && (path == "/api/base-layers" || strings.HasPrefix(path, "/api/base-layers/")) {
		user := GetUser(r)
		if user == nil {
			models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
			return
		}
		if path == "/api/base-layers" {
			rt.baseLayerH.ListOrCreateBaseLayers(w, r, user.ID)
			return
		}
		sub := strings.TrimPrefix(path, "/api/base-layers/")
		parts := strings.Split(sub, "/")
		if len(parts) == 1 && parts[0] != "" {
			rt.baseLayerH.BaseLayerItem(w, r, user.ID, parts[0])
			return
		}
		if len(parts) == 2 && parts[0] != "" {
			switch parts[1] {
			case "preview":
				rt.baseLayerH.BaseLayerPreview(w, r, user.ID, parts[0])
				return
			case "validate":
				rt.baseLayerH.ValidateBaseLayer(w, r, user.ID, parts[0])
				return
			}
		}
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Base layer endpoint not found", nil, nil)
		return
	}

	if rt.wsRegH != nil && path == "/api/datasets/inspect" {
		user := GetUser(r)
		if user == nil {
			models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
			return
		}
		rt.wsRegH.InspectDatasetPost(w, r, user.ID)
		return
	}

	if (rt.baseLayerH != nil || rt.wsRegH != nil) && strings.HasPrefix(path, "/api/workspaces/") {
		user := GetUser(r)
		if user == nil {
			models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
			return
		}
		sub := strings.TrimPrefix(path, "/api/workspaces/")
		parts := strings.Split(sub, "/")
		wsID := parts[0]
		if wsID == "" || len(parts) < 2 {
			models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Workspace endpoint not found", nil, nil)
			return
		}

		switch parts[1] {
		case "base-layer":
			if len(parts) == 2 && rt.baseLayerH != nil {
				rt.baseLayerH.WorkspaceBaseLayer(w, r, user.ID, wsID)
				return
			}
		case "map-view":
			if len(parts) == 2 && rt.baseLayerH != nil {
				rt.baseLayerH.WorkspaceMapView(w, r, user.ID, wsID)
				return
			}
		case "layers":
			if rt.wsRegH != nil {
				if len(parts) == 2 {
					rt.wsRegH.WorkspaceLayers(w, r, user.ID, wsID)
					return
				}
				if len(parts) == 3 {
					switch parts[2] {
					case "order":
						rt.wsRegH.WorkspaceLayersOrder(w, r, user.ID, wsID)
						return
					case "visibility":
						rt.wsRegH.WorkspaceLayersVisibility(w, r, user.ID, wsID)
						return
					default:
						rt.wsRegH.WorkspaceLayerItem(w, r, user.ID, wsID, parts[2])
						return
					}
				}
				if len(parts) == 4 && parts[3] == "features" {
					rt.wsRegH.WorkspaceLayerFeatures(w, r, user.ID, wsID, parts[2])
					return
				}
				if len(parts) == 5 && parts[3] == "features" {
					rt.wsRegH.WorkspaceLayerFeatureItem(w, r, user.ID, wsID, parts[2], parts[4], false)
					return
				}
				if len(parts) == 6 && parts[3] == "features" && parts[5] == "geometry" {
					rt.wsRegH.WorkspaceLayerFeatureItem(w, r, user.ID, wsID, parts[2], parts[4], true)
					return
				}
			}
		case "records":
			if rt.wsRegH != nil {
				if len(parts) == 2 {
					rt.wsRegH.WorkspaceRecords(w, r, user.ID, wsID)
					return
				}
				if len(parts) == 3 {
					switch parts[2] {
					case "counts":
						rt.wsRegH.WorkspaceRecordCounts(w, r, user.ID, wsID)
						return
					case "schema":
						rt.wsRegH.WorkspaceRecordSchema(w, r, user.ID, wsID)
						return
					case "export":
						rt.wsRegH.WorkspaceRecordsExport(w, r, user.ID, wsID)
						return
					default:
						rt.wsRegH.WorkspaceRecordItem(w, r, user.ID, wsID, parts[2])
						return
					}
				}
			}
		case "rasters":
			if rt.wsRegH != nil {
				if len(parts) == 2 {
					rt.wsRegH.WorkspaceRasters(w, r, user.ID, wsID)
					return
				}
				if len(parts) == 3 || (len(parts) == 4 && parts[3] == "metadata") {
					rt.wsRegH.WorkspaceRasterItem(w, r, user.ID, wsID, parts[2])
					return
				}
			}
		}
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Workspace endpoint not found", nil, nil)
		return
	}

	// 1a-maps. Personal and Shared Map Endpoints (/api/maps)
	if rt.mapsH != nil && (path == "/api/maps" || strings.HasPrefix(path, "/api/maps/")) {
		if path == "/api/maps/from-configuration" {
			user := GetUser(r)
			if user == nil {
				models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
				return
			}
			rt.mapsH.CreateMapFromConfiguration(w, r, user)
			return
		}
		if path == "/api/maps" {
			user := GetUser(r)
			if user == nil {
				models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
				return
			}
			if r.Method == http.MethodGet {
				rt.mapsH.ListMaps(w, r, user)
				return
			}
			if r.Method == http.MethodPost {
				rt.mapsH.CreateMap(w, r, user)
				return
			}
		}

		sub := strings.TrimPrefix(path, "/api/maps/")
		parts := strings.Split(sub, "/")
		if len(parts) == 1 {
			mapID := parts[0]
			user := GetUser(r)
			if user == nil {
				models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
				return
			}
			if r.Method == http.MethodGet {
				rt.mapsH.GetMap(w, r, user, mapID)
				return
			}
			if r.Method == http.MethodPut || r.Method == http.MethodPatch {
				rt.mapsH.UpdateMap(w, r, user, mapID)
				return
			}
			if r.Method == http.MethodDelete {
				rt.mapsH.DeleteMap(w, r, user, mapID)
				return
			}
		}

		if len(parts) >= 2 {
			mapID := parts[0]
			action := parts[1]

			if action == "lock" {
				if len(parts) == 2 {
					if r.Method == http.MethodGet {
						rt.mapsH.GetLock(w, r, mapID)
						return
					}
					user := GetUser(r)
					if user == nil {
						models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
						return
					}
					if r.Method == http.MethodPost {
						rt.mapsH.AcquireLock(w, r, user, mapID)
						return
					}
					if r.Method == http.MethodDelete {
						rt.mapsH.ReleaseLock(w, r, user, mapID)
						return
					}
				} else if len(parts) == 3 && parts[2] == "renew" {
					user := GetUser(r)
					if user == nil {
						models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
						return
					}
					if r.Method == http.MethodPost {
						rt.mapsH.RenewLock(w, r, user, mapID)
						return
					}
				}
			}

			user := GetUser(r)
			if user == nil {
				models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
				return
			}

			if action == "save" && r.Method == http.MethodPost {
				rt.mapsH.SaveMapConfig(w, r, user, mapID)
				return
			}
			if action == "recent" && r.Method == http.MethodPost {
				rt.mapsH.RecordRecentMap(w, r, user, mapID)
				return
			}
			if action == "saved" {
				if r.Method == http.MethodPost {
					rt.mapsH.SaveMapToLibrary(w, r, user, mapID)
					return
				}
				if r.Method == http.MethodDelete {
					rt.mapsH.RemoveMapFromLibrary(w, r, user, mapID)
					return
				}
			}
			if action == "revisions" {
				if len(parts) == 2 && r.Method == http.MethodGet {
					rt.mapsH.ListRevisions(w, r, user, mapID)
					return
				}
				if len(parts) == 4 && parts[3] == "restore" && r.Method == http.MethodPost {
					rt.mapsH.RestoreRevision(w, r, user, mapID, parts[2])
					return
				}
			}
			if action == "invitations" && r.Method == http.MethodPost {
				rt.mapsH.InviteToMap(w, r, user, mapID)
				return
			}
			if action == "shares" {
				if len(parts) == 2 && r.Method == http.MethodGet {
					rt.mapsH.ListShares(w, r, user, mapID)
					return
				}
				if len(parts) == 3 {
					shareID := parts[2]
					if r.Method == http.MethodPatch || r.Method == http.MethodPut {
						rt.mapsH.UpdateSharePermission(w, r, user, mapID, shareID)
						return
					}
					if r.Method == http.MethodDelete {
						rt.mapsH.RevokeShare(w, r, user, mapID, shareID)
						return
					}
				}
			}
			if action == "share-links" {
				if len(parts) == 2 {
					if r.Method == http.MethodPost {
						rt.mapsH.CreateShareLink(w, r, user, mapID)
						return
					}
					if r.Method == http.MethodGet {
						rt.mapsH.ListShareLinks(w, r, user, mapID)
						return
					}
				}
				if len(parts) == 3 && r.Method == http.MethodDelete {
					rt.mapsH.RevokeShareLink(w, r, user, mapID, parts[2])
					return
				}
			}
			if rt.gisH != nil && action == "layers" {
				if len(parts) == 2 {
					if r.Method == http.MethodGet {
						rt.gisH.GetLayers(w, r, user, mapID)
						return
					}
					if r.Method == http.MethodPost {
						rt.gisH.CreateLayer(w, r, user, mapID)
						return
					}
				}
				if len(parts) == 3 && parts[2] == "order" && (r.Method == http.MethodPut || r.Method == http.MethodPost) {
					rt.gisH.ReorderLayers(w, r, user, mapID)
					return
				}
			}
			if rt.gisH != nil && action == "layer-groups" && len(parts) == 2 && r.Method == http.MethodPost {
				rt.gisH.CreateLayerGroup(w, r, user, mapID)
				return
			}
		}
	}

	// 1a-maps-short. Support /maps/:mapId/layers, /maps/:mapId/layers/order, /maps/:mapId/layer-groups
	if rt.gisH != nil && strings.HasPrefix(path, "/maps/") {
		sub := strings.TrimPrefix(path, "/maps/")
		parts := strings.Split(sub, "/")
		if len(parts) >= 2 {
			mapID := parts[0]
			action := parts[1]
			user := GetUser(r)
			if action == "layers" {
				if len(parts) == 2 {
					if r.Method == http.MethodGet {
						rt.gisH.GetLayers(w, r, user, mapID)
						return
					}
					if r.Method == http.MethodPost {
						rt.gisH.CreateLayer(w, r, user, mapID)
						return
					}
				}
				if len(parts) == 3 && parts[2] == "order" && (r.Method == http.MethodPut || r.Method == http.MethodPost) {
					rt.gisH.ReorderLayers(w, r, user, mapID)
					return
				}
			}
			if action == "layer-groups" && len(parts) == 2 && r.Method == http.MethodPost {
				rt.gisH.CreateLayerGroup(w, r, user, mapID)
				return
			}
		}
	}

	// 1b. User Projects (Dashboard, Ownership, Sharing & Collaboration)
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
			rt.projectsH.Create(w, r, user)
			return
		}
	}

	if path == "/api/projects/shared" || path == "/projects/shared" {
		user := GetUser(r)
		if user == nil {
			models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
			return
		}
		rt.projectsH.ListShared(w, r, user.ID)
		return
	}

	if path == "/api/projects/invitations" || path == "/projects/invitations" {
		user := GetUser(r)
		if user == nil {
			models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
			return
		}
		rt.projectsH.ListInvitations(w, r, user)
		return
	}

	if (strings.HasPrefix(path, "/api/projects/invitations/") || strings.HasPrefix(path, "/projects/invitations/")) && strings.HasSuffix(path, "/respond") {
		user := GetUser(r)
		if user == nil {
			models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
			return
		}
		var remainder string
		if strings.HasPrefix(path, "/api/projects/invitations/") {
			remainder = strings.TrimPrefix(path, "/api/projects/invitations/")
		} else {
			remainder = strings.TrimPrefix(path, "/projects/invitations/")
		}
		invitationID := strings.TrimSuffix(remainder, "/respond")
		rt.projectsH.RespondInvitation(w, r, invitationID, user)
		return
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

		if len(parts) == 2 {
			projectID := parts[0]
			sub := parts[1]

			if sub == "share" && r.Method == http.MethodPost {
				rt.projectsH.Share(w, r, projectID, user)
				return
			}

			if sub == "members" && r.Method == http.MethodGet {
				rt.projectsH.ListMembers(w, r, projectID, user.ID)
				return
			}

			if sub == "activities" && r.Method == http.MethodGet {
				rt.projectsH.ListActivities(w, r, projectID, user.ID)
				return
			}

			if sub == "leave" && (r.Method == http.MethodPost || r.Method == http.MethodDelete) {
				rt.projectsH.Leave(w, r, projectID, user.ID)
				return
			}
		}

		if len(parts) == 3 && parts[1] == "members" {
			projectID := parts[0]
			memberID := parts[2]

			if r.Method == http.MethodPatch || r.Method == http.MethodPut {
				rt.projectsH.UpdateMemberRole(w, r, projectID, memberID, user)
				return
			}
			if r.Method == http.MethodDelete {
				rt.projectsH.RemoveMember(w, r, projectID, memberID, user)
				return
			}
		}
	}

	// 1c. Unified Invitations Endpoint (/api/invitations)
	if path == "/api/invitations" || path == "/invitations" {
		user := GetUser(r)
		if user == nil {
			models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
			return
		}
		if r.Method == http.MethodGet {
			projInvites := rt.projRepo.ListPendingInvitations(user.ID, user.Email, rt.authRepo)
			models.WriteJSON(w, http.StatusOK, map[string]interface{}{
				"project_invitations": projInvites,
				"total":               len(projInvites),
			})
			return
		}
	}

	if strings.HasPrefix(path, "/api/invitations/") || strings.HasPrefix(path, "/invitations/") {
		user := GetUser(r)
		if user == nil {
			models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
			return
		}
		var remainder string
		if strings.HasPrefix(path, "/api/invitations/") {
			remainder = strings.TrimPrefix(path, "/api/invitations/")
		} else {
			remainder = strings.TrimPrefix(path, "/invitations/")
		}
		parts := strings.Split(remainder, "/")
		if len(parts) == 2 {
			invitationID := parts[0]
			action := parts[1] // "accept" or "reject"
			if r.Method == http.MethodPost {
				accept := (action == "accept")
				err := rt.projRepo.RespondInvitation(invitationID, user.ID, user.Email, user.Name, accept)
				if err == nil {
					models.WriteJSON(w, http.StatusOK, map[string]interface{}{"success": true, "accepted": accept, "type": "project"})
					return
				}
				models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Invitation not found or already processed", nil, nil)
				return
			}
		}
	}

	// 2. Sessions
	if path == "/api/workspace/session/start" || path == "/workspace/session/start" || path == "/api/workspace/session" || path == "/workspace/session" {
		sess := rt.repo.CreateSession()
		w.Header().Set("X-Session-ID", sess.ID)
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"session_id": sess.ID,
			"created_at": sess.CreatedAt,
		})
		return
	}
	if path == "/api/workspace/session/end" || path == "/workspace/session/end" {
		sid := GetSessionID(r)
		if sid != "" {
			rt.repo.DeleteSession(sid)
			if rt.mapsH != nil && rt.mapsH.GetMapRepo() != nil {
				_ = rt.mapsH.GetMapRepo().ClearSessionRecentMaps(r.Context(), sid)
			}
		}
		models.WriteJSON(w, http.StatusOK, map[string]bool{"ended": true})
		return
	}

	if path == "/sessions" || path == "/api/sessions" {
		if r.Method == http.MethodGet {
			list := rt.repo.ListSessions()
			models.WriteJSON(w, http.StatusOK, map[string]interface{}{
				"sessions": list,
				"total":    len(list),
			})
			return
		}
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
			if rt.mapsH != nil && rt.mapsH.GetMapRepo() != nil {
				_ = rt.mapsH.GetMapRepo().ClearSessionRecentMaps(r.Context(), sid)
			}
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
				if rt.mapsH != nil && rt.mapsH.GetMapRepo() != nil {
					_ = rt.mapsH.GetMapRepo().ClearSessionRecentMaps(r.Context(), sid)
				}
				models.WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
				return
			}
		}

		if len(parts) == 2 && parts[1] == "end" && r.Method == http.MethodPost {
			rt.repo.DeleteSession(parts[0])
			if rt.mapsH != nil && rt.mapsH.GetMapRepo() != nil {
				_ = rt.mapsH.GetMapRepo().ClearSessionRecentMaps(r.Context(), parts[0])
			}
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
			models.WriteError(w, http.StatusGone, "SESSION_CONFIGS_RETIRED", "Session-only saved configurations are retired; save to the persistent map library", nil, nil)
			return
		}
	}

	// 3. Uploads & Project Reopen
	if path == "/upload" || path == "/api/upload" || path == "/datasets/import-zip" || path == "/api/datasets/import" {
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
			if (r.Method == http.MethodPatch || r.Method == http.MethodPut) && rt.gisH != nil {
				rt.gisH.RenameDataset(w, r, GetSessionID(r), datasetID)
				return
			}
			if r.Method == http.MethodDelete {
				if rt.gisH != nil && r.URL.Query().Get("force") != "true" {
					if refCount, err := rt.gisH.GetService().CheckDatasetDependencies(r.Context(), datasetID); err == nil && refCount > 0 {
						models.WriteError(w, http.StatusConflict, "DATASET_IN_USE", models.ErrDatasetInUse.Error(), map[string]interface{}{
							"dataset_id":        datasetID,
							"referenced_layers": refCount,
						}, nil)
						return
					}
				}
				rt.datasetsH.Delete(w, r, datasetID)
				return
			}
		}

		if len(parts) >= 2 && parts[1] == "export" {
			if len(parts) >= 3 && parts[2] == "progress" {
				rt.exportH.GetProgress(w, r, datasetID)
				return
			}
			if len(parts) >= 3 && parts[2] == "cancel" {
				rt.exportH.CancelExport(w, r, datasetID)
				return
			}
			if len(parts) == 2 {
				rt.exportH.Export(w, r, datasetID)
				return
			}
		}

		if len(parts) == 2 {
			sub := parts[1]
			switch sub {
			case "source-image":
				rt.datasetsH.SourceImage(w, r, datasetID)
				return
			case "inspect":
				if rt.gisH != nil && r.Method == http.MethodPost {
					rt.gisH.InspectDataset(w, r, GetSessionID(r), datasetID)
					return
				}
			case "metadata":
				if rt.gisH != nil && r.Method == http.MethodGet {
					rt.gisH.GetDatasetMetadata(w, r, GetSessionID(r), datasetID)
					return
				}
			case "dataset-layers":
				if rt.gisH != nil && r.Method == http.MethodGet {
					rt.gisH.GetDatasetLayers(w, r, GetSessionID(r), datasetID)
					return
				}
			case "reproject":
				if rt.gisH != nil && r.Method == http.MethodPost {
					rt.gisH.ReprojectDataset(w, r, GetUser(r), GetSessionID(r), datasetID)
					return
				}
			case "crs":
				if rt.gisH != nil && r.Method == http.MethodPost {
					rt.gisH.SetDatasetCRS(w, r, GetSessionID(r), datasetID)
					return
				}
			case "layers":
				if r.URL.Query().Get("inspect") == "true" && rt.gisH != nil {
					rt.gisH.GetDatasetLayers(w, r, GetSessionID(r), datasetID)
					return
				}
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

		// Local Vector Tiles endpoints:
		//   GET /datasets/{id}/vector-tiles/{layerSlug}/{z}/{x}/{y}.pbf
		//   GET /datasets/{id}/vector-tiles/{layerSlug}/metadata.json
		//   GET /datasets/{id}/vector-tiles/{layerSlug}/style.json
		if parts[1] == "vector-tiles" {
			if len(parts) == 4 && (parts[3] == "metadata.json" || parts[3] == "style.json") {
				rt.datasetsH.ServeVectorTileAsset(w, r, datasetID, parts[2], parts[3])
				return
			}
			if len(parts) == 6 {
				rt.datasetsH.ServeVectorTile(w, r, datasetID, parts[2], parts[3], parts[4], parts[5])
				return
			}
		}

		if len(parts) >= 5 && parts[1] == "vector" {
			if len(parts) == 5 {
				rt.datasetsH.ServeTile(w, r, datasetID, parts[2], parts[3], parts[4])
				return
			}
			if len(parts) == 6 {
				rt.datasetsH.ServeTile(w, r, datasetID, parts[3], parts[4], parts[5])
				return
			}
		}

		if len(parts) == 6 && parts[1] == "tiles" && parts[2] == "vector" {
			rt.datasetsH.ServeTile(w, r, datasetID, parts[3], parts[4], parts[5])
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

	// 8. Export download & progress
	if strings.HasPrefix(path, "/api/exports/") && strings.HasSuffix(path, "/download") {
		trimmed := strings.TrimPrefix(path, "/api/exports/")
		trimmed = strings.TrimSuffix(trimmed, "/download")
		exportID := strings.Trim(trimmed, "/")
		rt.exportH.Download(w, r, exportID)
		return
	}
	if strings.HasPrefix(path, "/api/exports/") && strings.HasSuffix(path, "/progress") {
		trimmed := strings.TrimPrefix(path, "/api/exports/")
		trimmed = strings.TrimSuffix(trimmed, "/progress")
		exportID := strings.Trim(trimmed, "/")
		rt.exportH.GetProgress(w, r, exportID)
		return
	}
	if strings.HasPrefix(path, "/api/exports/") && strings.HasSuffix(path, "/cancel") {
		trimmed := strings.TrimPrefix(path, "/api/exports/")
		trimmed = strings.TrimSuffix(trimmed, "/cancel")
		exportID := strings.Trim(trimmed, "/")
		rt.exportH.CancelExport(w, r, exportID)
		return
	}

	// 9. Phase 1 GIS Foundation Routes (Modules A, B, C, D, E, G)
	// Explicit route prefixes for contract verification:
	// /api/layers/ /layers/ /api/layer-groups/ /layer-groups/
	// /api/features/ /features/ /api/features/merge /features/merge /api/features/validate /features/validate /api/features/snap /features/snap
	// /api/crs /crs /api/crs/ /crs/
	// /api/spatial-queries /spatial-queries /api/spatial-queries/validate /spatial-queries/validate /api/spatial-queries/execute /spatial-queries/execute
	if rt.gisH != nil {
		cleanGISPath := path
		if strings.HasPrefix(cleanGISPath, "/api/") {
			cleanGISPath = strings.TrimPrefix(cleanGISPath, "/api")
		}

		// 9a. Layer Manager, Vector Features, Attribute Table, & Symbology (/layers/:layerId/...)
		if strings.HasPrefix(cleanGISPath, "/layers/") {
			user := GetUser(r)
			trimmed := strings.TrimPrefix(cleanGISPath, "/layers/")
			parts := strings.Split(trimmed, "/")
			layerID := parts[0]

			if len(parts) == 1 && layerID != "" {
				switch r.Method {
				case http.MethodGet:
					rt.gisH.GetLayer(w, r, user, layerID)
					return
				case http.MethodPatch, http.MethodPut:
					rt.gisH.UpdateLayer(w, r, user, layerID)
					return
				case http.MethodDelete:
					rt.gisH.DeleteLayer(w, r, user, layerID)
					return
				}
			}

			if len(parts) == 2 {
				sub := parts[1]
				switch sub {
				case "visibility":
					if r.Method == http.MethodPatch || r.Method == http.MethodPut {
						rt.gisH.UpdateLayerVisibility(w, r, user, layerID)
						return
					}
				case "opacity":
					if r.Method == http.MethodPatch || r.Method == http.MethodPut {
						rt.gisH.UpdateLayerOpacity(w, r, user, layerID)
						return
					}
				case "duplicate":
					if r.Method == http.MethodPost {
						rt.gisH.DuplicateLayer(w, r, user, layerID)
						return
					}
				case "features":
					if r.Method == http.MethodGet {
						rt.gisH.GetFeatures(w, r, user, layerID)
						return
					}
					if r.Method == http.MethodPost {
						rt.gisH.CreateFeature(w, r, user, layerID)
						return
					}
				case "attributes":
					if r.Method == http.MethodGet {
						rt.gisH.GetAttributeTable(w, r, user, layerID)
						return
					}
				case "fields":
					if r.Method == http.MethodPost {
						rt.gisH.AddAttributeField(w, r, user, layerID)
						return
					}
				case "style":
					if r.Method == http.MethodGet {
						rt.gisH.GetLayerStyle(w, r, user, layerID)
						return
					}
					if r.Method == http.MethodPut || r.Method == http.MethodPost {
						rt.gisH.UpdateLayerStyle(w, r, user, layerID)
						return
					}
					if r.Method == http.MethodDelete {
						rt.gisH.DeleteLayerStyle(w, r, user, layerID)
						return
					}
				}
			}

			if len(parts) == 3 {
				if parts[1] == "fields" && r.Method == http.MethodDelete {
					rt.gisH.DeleteAttributeField(w, r, user, layerID, parts[2])
					return
				}
				if parts[1] == "attributes" {
					switch parts[2] {
					case "filter":
						if r.Method == http.MethodPost {
							rt.gisH.FilterAttributes(w, r, user, layerID)
							return
						}
					case "calculate":
						if r.Method == http.MethodPost {
							rt.gisH.CalculateField(w, r, user, layerID)
							return
						}
					case "export":
						if r.Method == http.MethodGet || r.Method == http.MethodPost {
							rt.gisH.ExportAttributes(w, r, user, layerID)
							return
						}
					}
				}
				if parts[1] == "style" && parts[2] == "validate" && r.Method == http.MethodPost {
					rt.gisH.ValidateLayerStyle(w, r)
					return
				}
			}
		}

		// 9b. Layer Groups (/layer-groups/:groupId)
		if strings.HasPrefix(cleanGISPath, "/layer-groups/") {
			user := GetUser(r)
			groupID := strings.TrimPrefix(cleanGISPath, "/layer-groups/")
			if groupID != "" && !strings.Contains(groupID, "/") {
				if r.Method == http.MethodPatch || r.Method == http.MethodPut {
					rt.gisH.UpdateLayerGroup(w, r, user, groupID)
					return
				}
				if r.Method == http.MethodDelete {
					rt.gisH.DeleteLayerGroup(w, r, user, groupID)
					return
				}
			}
		}

		// 9c. Vector Features & Geometry Operations (/features/...)
		if cleanGISPath == "/features/merge" && r.Method == http.MethodPost {
			rt.gisH.MergeFeatures(w, r, GetUser(r))
			return
		}
		if cleanGISPath == "/features/validate" && r.Method == http.MethodPost {
			rt.gisH.ValidateGeometry(w, r)
			return
		}
		if cleanGISPath == "/features/snap" && r.Method == http.MethodPost {
			rt.gisH.SnapGeometry(w, r, GetUser(r))
			return
		}
		if strings.HasPrefix(cleanGISPath, "/features/") {
			user := GetUser(r)
			trimmed := strings.TrimPrefix(cleanGISPath, "/features/")
			parts := strings.Split(trimmed, "/")
			featureID := parts[0]

			if len(parts) == 1 && featureID != "" {
				switch r.Method {
				case http.MethodGet:
					rt.gisH.GetFeature(w, r, user, featureID)
					return
				case http.MethodPatch, http.MethodPut:
					rt.gisH.UpdateFeature(w, r, user, featureID)
					return
				case http.MethodDelete:
					rt.gisH.DeleteFeature(w, r, user, featureID)
					return
				}
			}
			if len(parts) == 2 {
				switch parts[1] {
				case "split":
					if r.Method == http.MethodPost {
						rt.gisH.SplitFeature(w, r, user, featureID)
						return
					}
				case "attributes":
					if r.Method == http.MethodPatch || r.Method == http.MethodPut {
						rt.gisH.UpdateFeatureAttributes(w, r, user, featureID)
						return
					}
				case "history":
					if r.Method == http.MethodGet {
						rt.gisH.GetFeatureHistory(w, r, user, featureID)
						return
					}
				}
			}
		}

		// 9d. CRS Management (/crs and /crs/:epsgCode)
		if cleanGISPath == "/crs" && r.Method == http.MethodGet {
			rt.gisH.ListSupportedCRS(w, r)
			return
		}
		if strings.HasPrefix(cleanGISPath, "/crs/") && r.Method == http.MethodGet {
			epsgCode := strings.TrimPrefix(cleanGISPath, "/crs/")
			if epsgCode != "" {
				rt.gisH.GetCRSInfo(w, r, epsgCode)
				return
			}
		}

		// 9e. Spatial Query Builder (/spatial-queries/...)
		if cleanGISPath == "/spatial-queries/validate" && r.Method == http.MethodPost {
			rt.gisH.ValidateSpatialQuery(w, r, GetUser(r))
			return
		}
		if cleanGISPath == "/spatial-queries/execute" && r.Method == http.MethodPost {
			rt.gisH.ExecuteSpatialQuery(w, r, GetUser(r))
			return
		}
		if cleanGISPath == "/spatial-queries" {
			user := GetUser(r)
			if r.Method == http.MethodGet {
				rt.gisH.GetSavedQueries(w, r, user)
				return
			}
			if r.Method == http.MethodPost {
				rt.gisH.SaveSpatialQuery(w, r, user)
				return
			}
		}
		if strings.HasPrefix(cleanGISPath, "/spatial-queries/") && r.Method == http.MethodDelete {
			queryID := strings.TrimPrefix(cleanGISPath, "/spatial-queries/")
			if queryID != "" && !strings.Contains(queryID, "/") {
				rt.gisH.DeleteSpatialQuery(w, r, GetUser(r), queryID)
				return
			}
		}
	}

	// =========================================================================
	// 10. Phase 2 GIS Processing Engine & Toolbox Routes
	// Explicit route prefixes for contract verification:
	// /api/processing/algorithms /processing/algorithms /api/processing/algorithms/ /processing/algorithms/
	// /api/processing/validate /processing/validate /api/processing/execute /processing/execute
	// /api/processing/presets /processing/presets /api/processing/presets/ /processing/presets/
	// /api/analysis/buffer /analysis/buffer /api/analysis/buffer/ /analysis/buffer/
	// /api/analysis/intersection /analysis/intersection /api/analysis/union /analysis/union
	// /api/analysis/difference /analysis/difference /api/analysis/clip /analysis/clip
	// /api/analysis/dissolve /analysis/dissolve /api/analysis/symmetrical-difference /analysis/symmetrical-difference
	// /api/analysis/spatial-join /analysis/spatial-join
	// /api/analysis/proximity /analysis/proximity /api/analysis/distance-matrix /analysis/distance-matrix /api/analysis/nearest-facility /analysis/nearest-facility
	// /api/analysis/heatmap /analysis/heatmap
	// /api/raster/clip /raster/clip /api/raster/reproject /raster/reproject /api/raster/resample /raster/resample
	// /api/raster/calculator /raster/calculator /api/raster/align /raster/align /api/raster/mosaic /raster/mosaic
	// /api/raster/contours /raster/contours /api/raster/hillshade /raster/hillshade
	// /api/analysis/topology /analysis/topology /api/analysis/topology/ /analysis/topology/
	// /api/jobs /jobs /api/jobs/ /jobs/
	// =========================================================================
	if rt.procH != nil {
		cleanProcPath := strings.TrimPrefix(path, "/api")
		user := GetUser(r)
		sessionID := GetSessionID(r)

		// 10a. Processing Toolbox Discovery & Generic Execution
		if cleanProcPath == "/processing/algorithms" && r.Method == http.MethodGet {
			rt.procH.GetAlgorithms(w, r)
			return
		}
		if strings.HasPrefix(cleanProcPath, "/processing/algorithms/") && r.Method == http.MethodGet {
			algID := strings.TrimPrefix(cleanProcPath, "/processing/algorithms/")
			if algID != "" {
				rt.procH.GetAlgorithmDetails(w, r, algID)
				return
			}
		}
		if cleanProcPath == "/processing/validate" && r.Method == http.MethodPost {
			rt.procH.ValidateProcessingRequest(w, r)
			return
		}
		if cleanProcPath == "/processing/execute" && r.Method == http.MethodPost {
			rt.procH.ExecuteProcessingJob(w, r, user, sessionID)
			return
		}
		if cleanProcPath == "/processing/presets" {
			if r.Method == http.MethodGet {
				rt.procH.ListPresets(w, r, user)
				return
			}
			if r.Method == http.MethodPost {
				rt.procH.SavePreset(w, r, user)
				return
			}
		}
		if strings.HasPrefix(cleanProcPath, "/processing/presets/") && r.Method == http.MethodDelete {
			presetID := strings.TrimPrefix(cleanProcPath, "/processing/presets/")
			if presetID != "" {
				rt.procH.DeletePreset(w, r, user, presetID)
				return
			}
		}

		// 10b. Buffer Analysis (/analysis/buffer & /analysis/buffer/:jobId)
		if cleanProcPath == "/analysis/buffer" && r.Method == http.MethodPost {
			rt.procH.BufferAnalysis(w, r, user, sessionID)
			return
		}
		if strings.HasPrefix(cleanProcPath, "/analysis/buffer/") && r.Method == http.MethodGet {
			jobID := strings.TrimPrefix(cleanProcPath, "/analysis/buffer/")
			if jobID != "" {
				rt.procH.GetBufferJob(w, r, user, jobID)
				return
			}
		}

		// 10c. Overlay Analysis
		if cleanProcPath == "/analysis/intersection" && r.Method == http.MethodPost {
			rt.procH.OverlayAnalysis(w, r, user, sessionID, "intersection")
			return
		}
		if cleanProcPath == "/analysis/union" && r.Method == http.MethodPost {
			rt.procH.OverlayAnalysis(w, r, user, sessionID, "union")
			return
		}
		if cleanProcPath == "/analysis/difference" && r.Method == http.MethodPost {
			rt.procH.OverlayAnalysis(w, r, user, sessionID, "difference")
			return
		}
		if cleanProcPath == "/analysis/clip" && r.Method == http.MethodPost {
			rt.procH.OverlayAnalysis(w, r, user, sessionID, "clip")
			return
		}
		if cleanProcPath == "/analysis/dissolve" && r.Method == http.MethodPost {
			rt.procH.DissolveAnalysis(w, r, user, sessionID)
			return
		}
		if cleanProcPath == "/analysis/symmetrical-difference" && r.Method == http.MethodPost {
			rt.procH.OverlayAnalysis(w, r, user, sessionID, "symmetrical_difference")
			return
		}

		// 10d. Spatial Join
		if cleanProcPath == "/analysis/spatial-join" && r.Method == http.MethodPost {
			rt.procH.SpatialJoin(w, r, user, sessionID)
			return
		}

		// 10e. Proximity Analysis
		if cleanProcPath == "/analysis/proximity" && r.Method == http.MethodPost {
			rt.procH.ProximityAnalysis(w, r, user, sessionID, "proximity")
			return
		}
		if cleanProcPath == "/analysis/distance-matrix" && r.Method == http.MethodPost {
			rt.procH.ProximityAnalysis(w, r, user, sessionID, "distance_matrix")
			return
		}
		if cleanProcPath == "/analysis/nearest-facility" && r.Method == http.MethodPost {
			rt.procH.ProximityAnalysis(w, r, user, sessionID, "nearest_facility")
			return
		}

		// 10f. Heatmap KDE Generation
		if cleanProcPath == "/analysis/heatmap" && r.Method == http.MethodPost {
			rt.procH.HeatmapGeneration(w, r, user, sessionID)
			return
		}

		// 10g. Raster Processing Toolbox
		if cleanProcPath == "/raster/clip" && r.Method == http.MethodPost {
			rt.procH.RasterClip(w, r, user, sessionID)
			return
		}
		if cleanProcPath == "/raster/reproject" && r.Method == http.MethodPost {
			rt.procH.RasterReproject(w, r, user, sessionID)
			return
		}
		if cleanProcPath == "/raster/resample" && r.Method == http.MethodPost {
			rt.procH.RasterResample(w, r, user, sessionID)
			return
		}
		if cleanProcPath == "/raster/calculator" && r.Method == http.MethodPost {
			rt.procH.RasterCalculator(w, r, user, sessionID)
			return
		}
		if cleanProcPath == "/raster/align" && r.Method == http.MethodPost {
			rt.procH.RasterAlign(w, r, user, sessionID)
			return
		}
		if cleanProcPath == "/raster/mosaic" && r.Method == http.MethodPost {
			rt.procH.RasterMosaic(w, r, user, sessionID)
			return
		}
		if cleanProcPath == "/raster/contours" && r.Method == http.MethodPost {
			rt.procH.RasterContours(w, r, user, sessionID)
			return
		}
		if cleanProcPath == "/raster/hillshade" && r.Method == http.MethodPost {
			rt.procH.RasterHillshade(w, r, user, sessionID)
			return
		}

		// 10h. Topology Validation & Fixes
		if cleanProcPath == "/analysis/topology" && r.Method == http.MethodPost {
			rt.procH.TopologyValidation(w, r, user, sessionID)
			return
		}
		if strings.HasPrefix(cleanProcPath, "/analysis/topology/") {
			sub := strings.TrimPrefix(cleanProcPath, "/analysis/topology/")
			parts := strings.Split(sub, "/")
			if len(parts) == 2 && parts[0] != "" {
				jobID := parts[0]
				switch parts[1] {
				case "results":
					if r.Method == http.MethodGet {
						rt.procH.GetTopologyResults(w, r, user, jobID)
						return
					}
				case "errors":
					if r.Method == http.MethodGet {
						rt.procH.GetTopologyErrors(w, r, user, jobID)
						return
					}
				case "fix":
					if r.Method == http.MethodPost {
						rt.procH.FixTopologyErrors(w, r, user, jobID)
						return
					}
				}
			}
		}

		// 10i. Processing Jobs Lifecycle (/jobs, /jobs/:jobId, /jobs/:jobId/cancel, /jobs/:jobId/result, /jobs/:jobId/logs)
		if cleanProcPath == "/jobs" && r.Method == http.MethodGet {
			rt.procH.ListJobs(w, r, user)
			return
		}
		if strings.HasPrefix(cleanProcPath, "/jobs/") {
			sub := strings.TrimPrefix(cleanProcPath, "/jobs/")
			parts := strings.Split(sub, "/")
			if len(parts) == 1 && parts[0] != "" && r.Method == http.MethodGet {
				rt.procH.GetJob(w, r, user, parts[0])
				return
			}
			if len(parts) == 2 && parts[0] != "" {
				jobID := parts[0]
				switch parts[1] {
				case "cancel":
					if r.Method == http.MethodPost {
						rt.procH.CancelJob(w, r, user, jobID)
						return
					}
				case "result":
					if r.Method == http.MethodGet {
						rt.procH.GetJobResult(w, r, user, jobID)
						return
					}
				case "logs":
					if r.Method == http.MethodGet {
						rt.procH.GetJobLogs(w, r, user, jobID)
						return
					}
				}
			}
		}
	}

	if serveSPA(w, r) {
		return
	}

	models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Endpoint not found: "+r.URL.Path, nil, nil)
}
