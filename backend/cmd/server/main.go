package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	appaudit "yunxia/internal/application/audit"
	appsvc "yunxia/internal/application/service"
	appcfg "yunxia/internal/infrastructure/config"
	"yunxia/internal/infrastructure/downloader"
	appLog "yunxia/internal/infrastructure/observability/logging"
	gormrepo "yunxia/internal/infrastructure/persistence/gorm"
	"yunxia/internal/infrastructure/security"
	infraStorage "yunxia/internal/infrastructure/storage"
	httpiface "yunxia/internal/interfaces/http"
	httphandler "yunxia/internal/interfaces/http/handler"
	mw "yunxia/internal/interfaces/middleware"
)

func main() {
	cfg, err := appcfg.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if err := prepareDirectories(cfg); err != nil {
		log.Fatalf("prepare directories: %v", err)
	}

	rootLogger := appLog.NewRootLogger(appLog.Options{
		Level:     cfg.Logging.Level,
		Format:    cfg.Logging.Format,
		AddSource: cfg.Logging.AddSource,
	}, appLog.AppMeta{
		Service: "yunxia-backend",
		Env:     cfg.Server.Mode,
		Version: "dev",
		Commit:  "local",
	}, os.Stdout, os.Stderr)
	slog.SetDefault(rootLogger)

	gin.SetMode(cfg.Server.Mode)

	db, err := gormrepo.OpenSQLite(cfg.Database.DSN)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("db.DB(): %v", err)
	}
	defer sqlDB.Close()

	userRepo := gormrepo.NewUserRepository(db)
	refreshRepo := gormrepo.NewRefreshTokenRepository(db)
	systemConfigRepo := gormrepo.NewSystemConfigRepository(db)
	sourceRepo := gormrepo.NewSourceRepository(db)
	uploadRepo := gormrepo.NewUploadSessionRepository(db)
	taskRepo := gormrepo.NewTaskRepository(db)
	trashRepo := gormrepo.NewTrashItemRepository(db)
	aclRepo := gormrepo.NewACLRuleRepository(db)
	shareRepo := gormrepo.NewShareRepository(db)
	auditRepo := gormrepo.NewAuditLogRepository(db)

	hasher := security.NewBcryptHasher(cfg.Security.BcryptCost)
	tokenSvc := security.NewJWTTokenService(cfg.JWT.Secret, cfg.JWT.AccessTokenExpire, cfg.JWT.RefreshTokenExpire)
	fileAccessSvc := security.NewFileAccessTokenService(cfg.JWT.Secret)
	auditRecorder := appaudit.NewRecorder(auditRepo, appLog.Component(rootLogger, "audit.recorder"))
	auditQuerySvc := appaudit.NewQueryService(auditRepo)
	downloadSvc := downloader.NewAria2Client(cfg.Aria2.RPCURL, cfg.Aria2.RPCSecret)
	s3Driver := infraStorage.NewS3Driver(infraStorage.NewS3ClientFactory())

	options := appsvc.DefaultSystemOptions()
	options.StorageDataDir = cfg.Storage.DataDir
	options.TempDir = cfg.Storage.TempDir
	options.DefaultChunkSize = cfg.Storage.DefaultChunkSize
	options.MaxUploadSize = cfg.Storage.MaxUploadSize
	options.WebDAVEnabled = cfg.WebDAV.Enabled
	options.WebDAVPrefix = cfg.WebDAV.Prefix

	setupSvc := appsvc.NewSetupService(
		userRepo,
		refreshRepo,
		systemConfigRepo,
		sourceRepo,
		hasher,
		tokenSvc,
		options,
		appsvc.WithSetupAuditRecorder(auditRecorder),
	)
	authSvc := appsvc.NewAuthService(userRepo, refreshRepo, hasher, tokenSvc)
	systemSvc := appsvc.NewSystemService(
		systemConfigRepo,
		options,
		appsvc.WithSystemAuditRecorder(auditRecorder),
		appsvc.WithSystemStatsDependencies(userRepo, sourceRepo, taskRepo),
		appsvc.WithSystemStatsFileDriver("s3", s3Driver),
	)
	aclAuthorizer := appsvc.NewACLAuthorizer(systemConfigRepo, aclRepo, sourceRepo)
	sourceSvc := appsvc.NewSourceService(
		sourceRepo,
		systemConfigRepo,
		appsvc.WithSourceAuditRecorder(auditRecorder),
		appsvc.WithSourceACLAuthorizer(aclAuthorizer),
		appsvc.WithSourceDriverProbe("s3", s3Driver),
	)
	userSvc := appsvc.NewUserService(userRepo, hasher, appsvc.WithUserAuditRecorder(auditRecorder))
	aclSvc := appsvc.NewACLService(sourceRepo, userRepo, aclRepo, appsvc.WithACLAuditRecorder(auditRecorder))
	fileSvc := appsvc.NewFileService(
		sourceRepo,
		fileAccessSvc,
		tokenSvc,
		userRepo,
		appsvc.WithFileAuditRecorder(auditRecorder),
		appsvc.WithFileACLAuthorizer(aclAuthorizer),
		appsvc.WithFileDriver("s3", s3Driver),
		appsvc.WithTrashItemRepository(trashRepo),
	)
	trashSvc := appsvc.NewTrashService(
		sourceRepo,
		trashRepo,
		appsvc.WithTrashAuditRecorder(auditRecorder),
		appsvc.WithTrashACLAuthorizer(aclAuthorizer),
		appsvc.WithTrashFileDriver("s3", s3Driver),
	)
	vfsSvc := appsvc.NewVFSService(
		sourceRepo,
		appsvc.WithVFSFileDriver("s3", s3Driver),
		appsvc.WithVFSFileOperator(fileSvc),
	)
	uploadSvc := appsvc.NewUploadService(
		sourceRepo,
		uploadRepo,
		options,
		appsvc.WithUploadAuditRecorder(auditRecorder),
		appsvc.WithUploadACLAuthorizer(aclAuthorizer),
		appsvc.WithUploadDriver("s3", s3Driver),
		appsvc.WithUploadVFSResolver(vfsSvc),
	)
	taskSvc := appsvc.NewTaskService(
		taskRepo,
		sourceRepo,
		downloadSvc,
		appsvc.WithTaskAuditRecorder(auditRecorder),
		appsvc.WithTaskACLAuthorizer(aclAuthorizer),
		appsvc.WithTaskStagingDir(filepath.Join(cfg.Storage.TempDir, "downloads")),
		appsvc.WithTaskImportDriver("s3", s3Driver),
		appsvc.WithTaskVFSResolver(vfsSvc),
	)
	go taskSvc.StartSyncWorker(context.Background(), 5*time.Second)
	shareSvc := appsvc.NewShareService(
		shareRepo,
		sourceRepo,
		hasher,
		fileAccessSvc,
		appsvc.WithShareAuditRecorder(auditRecorder),
		appsvc.WithShareACLAuthorizer(aclAuthorizer),
		appsvc.WithShareFileDriver("s3", s3Driver),
	)

	setupHandler := httphandler.NewSetupHandler(setupSvc)
	authHandler := httphandler.NewAuthHandler(authSvc)
	systemHandler := httphandler.NewSystemHandler(systemSvc, "dev", "local", "", "")
	auditHandler := httphandler.NewAuditHandler(auditQuerySvc)
	sourceHandler := httphandler.NewSourceHandler(sourceSvc)
	userHandler := httphandler.NewUserHandler(userSvc)
	aclHandler := httphandler.NewACLHandler(aclSvc)
	fileHandler := httphandler.NewFileHandler(fileSvc)
	trashHandler := httphandler.NewTrashHandler(trashSvc)
	uploadHandler := httphandler.NewUploadHandler(uploadSvc)
	taskHandler := httphandler.NewTaskHandler(taskSvc)
	shareHandler := httphandler.NewShareHandler(shareSvc)
	vfsHandler := httphandler.NewVFSHandler(vfsSvc, fileSvc)
	webdavHandler := httphandler.NewWebDAVHandler(
		cfg.WebDAV.Prefix,
		sourceRepo,
		systemConfigRepo,
		userRepo,
		aclAuthorizer,
		hasher,
		auditRecorder,
		appLog.Component(rootLogger, "http.webdav"),
	)
	authMW := mw.NewAuthMiddleware(userRepo, tokenSvc)

	engine := httpiface.NewRouter(setupHandler, authHandler, systemHandler, authMW, rootLogger, cfg.WebDAV.Prefix, cfg.Logging.AccessLogEnabled)
	httpiface.RegisterStorageRoutes(engine, sourceHandler, fileHandler, trashHandler, uploadHandler, authMW, auditRecorder, rootLogger)
	httpiface.RegisterUserRoutes(engine, userHandler, authMW, auditRecorder, rootLogger)
	httpiface.RegisterACLRoutes(engine, aclHandler, authMW, auditRecorder, rootLogger)
	httpiface.RegisterAuditRoutes(engine, auditHandler, authMW, auditRecorder, rootLogger)
	httpiface.RegisterTaskRoutes(engine, taskHandler, authMW)
	httpiface.RegisterShareRoutes(engine, shareHandler, authMW)
	httpiface.RegisterVFSRoutes(engine, vfsHandler, authMW)
	httpiface.RegisterWebDAVRoutes(engine, cfg.WebDAV.Prefix, webdavHandler)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	rootLogger.Info("yunxia backend listening", slog.String("event", "app.start"), slog.String("addr", addr))
	if err := engine.Run(addr); err != nil {
		rootLogger.Error("run server failed", slog.String("event", "app.stop"), slog.Any("error", err))
		log.Fatalf("run server: %v", err)
	}
}

func prepareDirectories(cfg appcfg.Config) error {
	for _, dir := range []string{cfg.Storage.DataDir, cfg.Storage.TempDir} {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	if dsn := strings.TrimSpace(cfg.Database.DSN); dsn != "" && dsn != ":memory:" && !strings.HasPrefix(dsn, "file:") {
		if err := os.MkdirAll(filepath.Dir(dsn), 0o755); err != nil {
			return err
		}
	}
	return nil
}
