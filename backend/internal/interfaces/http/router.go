package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

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
	sourceTest.Use(middleware.RequireCapability(permission.CapabilitySourceTest))
	sourceTest.POST("/sources/test", sourceHandler.Test)
	sourceTest.POST("/sources/:id/test", sourceHandler.Retest)

	sourceCreate := authorized.Group("")
	sourceCreate.Use(middleware.RequireCapability(permission.CapabilitySourceCreate))
	sourceCreate.POST("/sources", sourceHandler.Create)

	sourceUpdate := authorized.Group("")
	sourceUpdate.Use(middleware.RequireCapability(permission.CapabilitySourceUpdate))
	sourceUpdate.PUT("/sources/:id", sourceHandler.Update)

	sourceDelete := authorized.Group("")
	sourceDelete.Use(middleware.RequireCapability(permission.CapabilitySourceDelete))
	sourceDelete.DELETE("/sources/:id", sourceHandler.Delete)

	api.GET("/files/download", fileHandler.Download)
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

	userRead := authorized.Group("")
	userRead.Use(middleware.RequireCapability(permission.CapabilityUserRead))
	userRead.GET("/users", userHandler.List)

	userCreate := authorized.Group("")
	userCreate.Use(
		middleware.RequireCapability(permission.CapabilityUserCreate),
		middleware.RequireCapability(permission.CapabilityUserRoleAssign),
	)
	userCreate.POST("/users", userHandler.Create)

	userUpdate := authorized.Group("")
	userUpdate.Use(
		middleware.RequireCapability(permission.CapabilityUserUpdate),
		middleware.RequireCapability(permission.CapabilityUserRoleAssign),
		middleware.RequireCapability(permission.CapabilityUserLock),
	)
	userUpdate.PUT("/users/:id", userHandler.Update)

	userReset := authorized.Group("")
	userReset.Use(middleware.RequireCapability(permission.CapabilityUserPasswordReset))
	userReset.POST("/users/:id/reset-password", userHandler.ResetPassword)

	userRevoke := authorized.Group("")
	userRevoke.Use(middleware.RequireCapability(permission.CapabilityUserTokensRevoke))
	userRevoke.POST("/users/:id/revoke-tokens", userHandler.RevokeTokens)
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

	aclRead := authorized.Group("")
	aclRead.Use(middleware.RequireCapability(permission.CapabilityACLRead))
	aclRead.GET("/acl/rules", aclHandler.List)

	aclManage := authorized.Group("")
	aclManage.Use(middleware.RequireCapability(permission.CapabilityACLManage))
	aclManage.POST("/acl/rules", aclHandler.Create)
	aclManage.PUT("/acl/rules/:id", aclHandler.Update)
	aclManage.DELETE("/acl/rules/:id", aclHandler.Delete)
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
