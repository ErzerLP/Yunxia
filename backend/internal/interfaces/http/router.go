package http

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	appaudit "yunxia/internal/application/audit"
	"yunxia/internal/domain/permission"
	"yunxia/internal/interfaces/http/handler"
	"yunxia/internal/interfaces/middleware"
)

// NewRouter 组装基础 HTTP 路由。
func NewRouter(
	setupHandler *handler.SetupHandler,
	authHandler *handler.AuthHandler,
	systemHandler *handler.SystemHandler,
	authMiddleware *middleware.AuthMiddleware,
	rootLogger *slog.Logger,
	webdavPrefix string,
	accessLogEnabled bool,
) *gin.Engine {
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.SecurityHeaders())
	if accessLogEnabled {
		r.Use(middleware.AccessLog(rootLogger, webdavPrefix, map[string]struct{}{
			"/api/v1/health": {},
		}))
	}
	r.Use(middleware.RecoveryWithLogger(rootLogger))

	api := r.Group("/api/v1")
	api.GET("/health", systemHandler.Health)
	api.GET("/setup/status", setupHandler.Status)
	api.POST("/setup/init", setupHandler.Init)
	api.POST("/auth/login", authHandler.Login)
	api.POST("/auth/refresh", authHandler.Refresh)

	authorized := api.Group("")
	authorized.Use(authMiddleware.RequireAuth())
	authorized.GET("/auth/me", authHandler.Me)
	authorized.POST("/auth/logout", authHandler.Logout)
	authorized.GET("/system/version", systemHandler.Version)

	statsGroup := authorized.Group("")
	statsGroup.Use(middleware.RequireCapability(permission.CapabilitySystemStatsRead))
	statsGroup.GET("/system/stats", systemHandler.Stats)

	configRead := authorized.Group("")
	configRead.Use(middleware.RequireCapability(permission.CapabilitySystemConfigRead))
	configRead.GET("/system/config", systemHandler.GetConfig)

	configWrite := authorized.Group("")
	configWrite.Use(middleware.RequireCapability(permission.CapabilitySystemConfigWrite))
	configWrite.PUT("/system/config", systemHandler.UpdateConfig)

	return r
}

// RegisterStorageRoutes 注册存储源、文件和上传相关路由。
func RegisterStorageRoutes(
	r *gin.Engine,
	sourceHandler *handler.SourceHandler,
	fileHandler *handler.FileHandler,
	trashHandler *handler.TrashHandler,
	uploadHandler *handler.UploadHandler,
	authMiddleware *middleware.AuthMiddleware,
	auditRecorder *appaudit.Recorder,
	rootLogger *slog.Logger,
) {
	api := r.Group("/api/v1")

	authorized := api.Group("")
	authorized.Use(authMiddleware.RequireAuth())
	authorized.GET("/sources", sourceHandler.List)
	authorized.GET("/files", fileHandler.List)
	authorized.GET("/files/search", fileHandler.Search)
	authorized.POST("/files/mkdir", fileHandler.Mkdir)
	authorized.POST("/files/rename", fileHandler.Rename)
	authorized.POST("/files/move", fileHandler.Move)
	authorized.POST("/files/copy", fileHandler.Copy)
	authorized.DELETE("/files", fileHandler.Delete)
	authorized.POST("/files/access-url", fileHandler.AccessURL)
	authorized.GET("/trash", trashHandler.List)
	authorized.POST("/trash/:id/restore", trashHandler.Restore)
	authorized.DELETE("/trash/:id", trashHandler.Delete)
	authorized.DELETE("/trash", trashHandler.Clear)
	authorized.POST("/upload/init", uploadHandler.Init)
	authorized.PUT("/upload/chunk", uploadHandler.UploadChunk)
	authorized.POST("/upload/finish", uploadHandler.Finish)
	authorized.GET("/upload/sessions", uploadHandler.ListSessions)
	authorized.DELETE("/upload/sessions/:upload_id", uploadHandler.Cancel)

	sourceRead := authorized.Group("")
	sourceRead.Use(middleware.RequireCapability(permission.CapabilitySourceRead))
	sourceRead.GET("/sources/:id", sourceHandler.Get)

	sourceTest := authorized.Group("")
	sourceTest.Use(middleware.RequireCapabilitiesForAction(auditRecorder, rootLogger, "storage_source", "test", permission.CapabilitySourceTest))
	sourceTest.POST("/sources/test", sourceHandler.Test)
	sourceTest.POST("/sources/:id/test", sourceHandler.Retest)

	sourceCreate := authorized.Group("")
	sourceCreate.Use(middleware.RequireCapabilitiesForAction(auditRecorder, rootLogger, "storage_source", "create", permission.CapabilitySourceCreate))
	sourceCreate.POST("/sources", sourceHandler.Create)

	sourceUpdate := authorized.Group("")
	sourceUpdate.Use(middleware.RequireCapabilitiesForAction(auditRecorder, rootLogger, "storage_source", "update", permission.CapabilitySourceUpdate))
	sourceUpdate.PUT("/sources/:id", sourceHandler.Update)

	sourceDelete := authorized.Group("")
	sourceDelete.Use(middleware.RequireCapabilitiesForAction(auditRecorder, rootLogger, "storage_source", "delete", permission.CapabilitySourceDelete))
	sourceDelete.DELETE("/sources/:id", sourceHandler.Delete)

	api.GET("/files/download", fileHandler.Download)
}

// RegisterVFSRoutes 注册统一虚拟目录树 V2 路由。
func RegisterVFSRoutes(
	r *gin.Engine,
	vfsHandler *handler.VFSHandler,
	authMiddleware *middleware.AuthMiddleware,
) {
	api := r.Group("/api/v2")

	authorized := api.Group("")
	authorized.Use(authMiddleware.RequireAuth())
	authorized.GET("/fs/list", vfsHandler.List)
	authorized.GET("/fs/search", vfsHandler.Search)
	authorized.POST("/fs/mkdir", vfsHandler.Mkdir)
	authorized.POST("/fs/rename", vfsHandler.Rename)
	authorized.POST("/fs/move", vfsHandler.Move)
	authorized.POST("/fs/copy", vfsHandler.Copy)
	authorized.DELETE("/fs", vfsHandler.Delete)
	authorized.POST("/fs/access-url", vfsHandler.AccessURL)

	api.GET("/fs/download", vfsHandler.Download)
}

// RegisterUserRoutes 注册用户管理相关路由。
func RegisterUserRoutes(
	r *gin.Engine,
	userHandler *handler.UserHandler,
	authMiddleware *middleware.AuthMiddleware,
	auditRecorder *appaudit.Recorder,
	rootLogger *slog.Logger,
) {
	api := r.Group("/api/v1")

	authorized := api.Group("")
	authorized.Use(authMiddleware.RequireAuth())

	userRead := authorized.Group("")
	userRead.Use(middleware.RequireCapability(permission.CapabilityUserRead))
	userRead.GET("/users", userHandler.List)

	userCreate := authorized.Group("")
	userCreate.Use(middleware.RequireCapabilitiesForAction(auditRecorder, rootLogger, "user", "create", permission.CapabilityUserCreate, permission.CapabilityUserRoleAssign))
	userCreate.POST("/users", userHandler.Create)

	userUpdate := authorized.Group("")
	userUpdate.Use(middleware.RequireCapabilitiesForAction(auditRecorder, rootLogger, "user", "update", permission.CapabilityUserUpdate, permission.CapabilityUserRoleAssign, permission.CapabilityUserLock))
	userUpdate.PUT("/users/:id", userHandler.Update)

	userReset := authorized.Group("")
	userReset.Use(middleware.RequireCapabilitiesForAction(auditRecorder, rootLogger, "user", "reset_password", permission.CapabilityUserPasswordReset))
	userReset.POST("/users/:id/reset-password", userHandler.ResetPassword)

	userRevoke := authorized.Group("")
	userRevoke.Use(middleware.RequireCapabilitiesForAction(auditRecorder, rootLogger, "user", "revoke_tokens", permission.CapabilityUserTokensRevoke))
	userRevoke.POST("/users/:id/revoke-tokens", userHandler.RevokeTokens)
}

// RegisterACLRoutes 注册 ACL 管理相关路由。
func RegisterACLRoutes(
	r *gin.Engine,
	aclHandler *handler.ACLHandler,
	authMiddleware *middleware.AuthMiddleware,
	auditRecorder *appaudit.Recorder,
	rootLogger *slog.Logger,
) {
	api := r.Group("/api/v1")

	authorized := api.Group("")
	authorized.Use(authMiddleware.RequireAuth())

	aclRead := authorized.Group("")
	aclRead.Use(middleware.RequireCapability(permission.CapabilityACLRead))
	aclRead.GET("/acl/rules", aclHandler.List)

	aclCreate := authorized.Group("")
	aclCreate.Use(middleware.RequireCapabilitiesForAction(auditRecorder, rootLogger, "acl_rule", "create", permission.CapabilityACLManage))
	aclCreate.POST("/acl/rules", aclHandler.Create)

	aclUpdate := authorized.Group("")
	aclUpdate.Use(middleware.RequireCapabilitiesForAction(auditRecorder, rootLogger, "acl_rule", "update", permission.CapabilityACLManage))
	aclUpdate.PUT("/acl/rules/:id", aclHandler.Update)

	aclDelete := authorized.Group("")
	aclDelete.Use(middleware.RequireCapabilitiesForAction(auditRecorder, rootLogger, "acl_rule", "delete", permission.CapabilityACLManage))
	aclDelete.DELETE("/acl/rules/:id", aclHandler.Delete)
}

// RegisterTaskRoutes 注册离线任务相关路由。
func RegisterTaskRoutes(
	r *gin.Engine,
	taskHandler *handler.TaskHandler,
	authMiddleware *middleware.AuthMiddleware,
) {
	api := r.Group("/api/v1")

	authorized := api.Group("")
	authorized.Use(authMiddleware.RequireAuth())
	authorized.GET("/tasks", taskHandler.List)
	authorized.POST("/tasks", taskHandler.Create)
	authorized.GET("/tasks/:id", taskHandler.Get)
	authorized.POST("/tasks/:id/pause", taskHandler.Pause)
	authorized.POST("/tasks/:id/resume", taskHandler.Resume)
	authorized.DELETE("/tasks/:id", taskHandler.Cancel)
}

// RegisterShareRoutes 注册分享相关路由。
func RegisterShareRoutes(
	r *gin.Engine,
	shareHandler *handler.ShareHandler,
	authMiddleware *middleware.AuthMiddleware,
) {
	api := r.Group("/api/v1")

	authorized := api.Group("")
	authorized.Use(authMiddleware.RequireAuth())
	authorized.GET("/shares", shareHandler.List)
	authorized.GET("/shares/:id", shareHandler.Get)
	authorized.POST("/shares", shareHandler.Create)
	authorized.PUT("/shares/:id", shareHandler.Update)
	authorized.DELETE("/shares/:id", shareHandler.Delete)

	r.GET("/s/:token", shareHandler.Open)
}

// RegisterWebDAVRoutes 注册 WebDAV 路由。
func RegisterWebDAVRoutes(
	r *gin.Engine,
	prefix string,
	webdavHandler *handler.WebDAVHandler,
) {
	normalizedPrefix := normalizeWebDAVPrefix(prefix)
	methods := []string{
		http.MethodOptions,
		http.MethodHead,
		http.MethodGet,
		http.MethodPut,
		http.MethodDelete,
		"PROPFIND",
		"MKCOL",
		"COPY",
		"MOVE",
	}

	exactPath := normalizedPrefix + "/:slug"
	wildcardPath := normalizedPrefix + "/:slug/*filepath"
	for _, method := range methods {
		r.Handle(method, exactPath, webdavHandler.Serve)
		r.Handle(method, wildcardPath, webdavHandler.Serve)
	}
}

func normalizeWebDAVPrefix(prefix string) string {
	if prefix == "" {
		return "/dav"
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if len(prefix) > 1 {
		prefix = strings.TrimRight(prefix, "/")
	}
	return prefix
}
