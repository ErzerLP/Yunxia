package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	appsvc "yunxia/internal/application/service"
	appcfg "yunxia/internal/infrastructure/config"
	"yunxia/internal/infrastructure/downloader"
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

	hasher := security.NewBcryptHasher(cfg.Security.BcryptCost)
	tokenSvc := security.NewJWTTokenService(cfg.JWT.Secret, cfg.JWT.AccessTokenExpire, cfg.JWT.RefreshTokenExpire)
	fileAccessSvc := security.NewFileAccessTokenService(cfg.JWT.Secret)
	downloadSvc := downloader.NewAria2Client(cfg.Aria2.RPCURL, cfg.Aria2.RPCSecret)
	s3Driver := infraStorage.NewS3Driver(infraStorage.NewS3ClientFactory())

	options := appsvc.DefaultSystemOptions()
	options.StorageDataDir = cfg.Storage.DataDir
	options.TempDir = cfg.Storage.TempDir
	options.DefaultChunkSize = cfg.Storage.DefaultChunkSize
	options.MaxUploadSize = cfg.Storage.MaxUploadSize
	options.WebDAVEnabled = cfg.WebDAV.Enabled
	options.WebDAVPrefix = cfg.WebDAV.Prefix

	setupSvc := appsvc.NewSetupService(userRepo, refreshRepo, systemConfigRepo, sourceRepo, hasher, tokenSvc, options)
	authSvc := appsvc.NewAuthService(userRepo, refreshRepo, hasher, tokenSvc)
	systemSvc := appsvc.NewSystemService(
		systemConfigRepo,
		options,
		appsvc.WithSystemStatsDependencies(userRepo, sourceRepo, taskRepo),
		appsvc.WithSystemStatsFileDriver("s3", s3Driver),
	)
	aclAuthorizer := appsvc.NewACLAuthorizer(systemConfigRepo, aclRepo)
	sourceSvc := appsvc.NewSourceService(
		sourceRepo,
		systemConfigRepo,
		appsvc.WithSourceACLAuthorizer(aclAuthorizer),
		appsvc.WithSourceDriverProbe("s3", s3Driver),
	)
	userSvc := appsvc.NewUserService(userRepo, hasher)
	aclSvc := appsvc.NewACLService(sourceRepo, userRepo, aclRepo)
	fileSvc := appsvc.NewFileService(
		sourceRepo,
		fileAccessSvc,
		tokenSvc,
		userRepo,
		appsvc.WithFileACLAuthorizer(aclAuthorizer),
		appsvc.WithFileDriver("s3", s3Driver),
		appsvc.WithTrashItemRepository(trashRepo),
	)
	trashSvc := appsvc.NewTrashService(
		sourceRepo,
		trashRepo,
		appsvc.WithTrashACLAuthorizer(aclAuthorizer),
		appsvc.WithTrashFileDriver("s3", s3Driver),
	)
	uploadSvc := appsvc.NewUploadService(
		sourceRepo,
		uploadRepo,
		options,
		appsvc.WithUploadACLAuthorizer(aclAuthorizer),
		appsvc.WithUploadDriver("s3", s3Driver),
	)
	taskSvc := appsvc.NewTaskService(taskRepo, sourceRepo, downloadSvc, appsvc.WithTaskACLAuthorizer(aclAuthorizer))
	shareSvc := appsvc.NewShareService(
		shareRepo,
		sourceRepo,
		hasher,
		fileAccessSvc,
		appsvc.WithShareACLAuthorizer(aclAuthorizer),
		appsvc.WithShareFileDriver("s3", s3Driver),
	)
	vfsSvc := appsvc.NewVFSService(
		sourceRepo,
		appsvc.WithVFSFileDriver("s3", s3Driver),
	)

	setupHandler := httphandler.NewSetupHandler(setupSvc)
	authHandler := httphandler.NewAuthHandler(authSvc)
	systemHandler := httphandler.NewSystemHandler(systemSvc, "dev", "local", "", "")
	sourceHandler := httphandler.NewSourceHandler(sourceSvc)
	userHandler := httphandler.NewUserHandler(userSvc)
	aclHandler := httphandler.NewACLHandler(aclSvc)
	fileHandler := httphandler.NewFileHandler(fileSvc)
	trashHandler := httphandler.NewTrashHandler(trashSvc)
	uploadHandler := httphandler.NewUploadHandler(uploadSvc)
	taskHandler := httphandler.NewTaskHandler(taskSvc)
	shareHandler := httphandler.NewShareHandler(shareSvc)
	vfsHandler := httphandler.NewVFSHandler(vfsSvc, fileSvc)
	webdavHandler := httphandler.NewWebDAVHandler(cfg.WebDAV.Prefix, sourceRepo, systemConfigRepo, userRepo, aclAuthorizer, hasher)
	authMW := mw.NewAuthMiddleware(userRepo, tokenSvc)

	engine := httpiface.NewRouter(setupHandler, authHandler, systemHandler, authMW)
	httpiface.RegisterStorageRoutes(engine, sourceHandler, fileHandler, trashHandler, uploadHandler, authMW)
	httpiface.RegisterUserRoutes(engine, userHandler, authMW)
	httpiface.RegisterACLRoutes(engine, aclHandler, authMW)
	httpiface.RegisterTaskRoutes(engine, taskHandler, authMW)
	httpiface.RegisterShareRoutes(engine, shareHandler, authMW)
	httpiface.RegisterVFSRoutes(engine, vfsHandler, authMW)
	httpiface.RegisterWebDAVRoutes(engine, cfg.WebDAV.Prefix, webdavHandler)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("yunxia backend listening on %s", addr)
	if err := engine.Run(addr); err != nil {
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
