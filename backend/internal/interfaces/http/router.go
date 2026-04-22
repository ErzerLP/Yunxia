package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"yunxia/internal/interfaces/http/handler"
	"yunxia/internal/interfaces/middleware"
)

// NewRouter 组装基础 HTTP 路由。
func NewRouter(
	setupHandler *handler.SetupHandler,
	authHandler *handler.AuthHandler,
	systemHandler *handler.SystemHandler,
	authMiddleware *middleware.AuthMiddleware,
) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestID(), middleware.SecurityHeaders())

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

	adminOnly := authorized.Group("")
	adminOnly.Use(middleware.RequireAdmin())
	adminOnly.GET("/system/config", systemHandler.GetConfig)
	adminOnly.GET("/system/stats", systemHandler.Stats)
	adminOnly.PUT("/system/config", systemHandler.UpdateConfig)

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

	adminOnly := authorized.Group("")
	adminOnly.Use(middleware.RequireAdmin())
	adminOnly.GET("/sources/:id", sourceHandler.Get)
	adminOnly.POST("/sources/test", sourceHandler.Test)
	adminOnly.POST("/sources", sourceHandler.Create)
	adminOnly.PUT("/sources/:id", sourceHandler.Update)
	adminOnly.DELETE("/sources/:id", sourceHandler.Delete)
	adminOnly.POST("/sources/:id/test", sourceHandler.Retest)

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
) {
	api := r.Group("/api/v1")

	authorized := api.Group("")
	authorized.Use(authMiddleware.RequireAuth())

	adminOnly := authorized.Group("")
	adminOnly.Use(middleware.RequireAdmin())
	adminOnly.GET("/users", userHandler.List)
	adminOnly.POST("/users", userHandler.Create)
	adminOnly.PUT("/users/:id", userHandler.Update)
	adminOnly.POST("/users/:id/reset-password", userHandler.ResetPassword)
	adminOnly.POST("/users/:id/revoke-tokens", userHandler.RevokeTokens)
}

// RegisterACLRoutes 注册 ACL 管理相关路由。
func RegisterACLRoutes(
	r *gin.Engine,
	aclHandler *handler.ACLHandler,
	authMiddleware *middleware.AuthMiddleware,
) {
	api := r.Group("/api/v1")

	authorized := api.Group("")
	authorized.Use(authMiddleware.RequireAuth())

	adminOnly := authorized.Group("")
	adminOnly.Use(middleware.RequireAdmin())
	adminOnly.GET("/acl/rules", aclHandler.List)
	adminOnly.POST("/acl/rules", aclHandler.Create)
	adminOnly.PUT("/acl/rules/:id", aclHandler.Update)
	adminOnly.DELETE("/acl/rules/:id", aclHandler.Delete)
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
