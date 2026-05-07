package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	slashpath "path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	appaudit "yunxia/internal/application/audit"
	appsvc "yunxia/internal/application/service"
	"yunxia/internal/domain/entity"
	appcfg "yunxia/internal/infrastructure/config"
	"yunxia/internal/infrastructure/downloader"
	infraNotification "yunxia/internal/infrastructure/notification"
	appLog "yunxia/internal/infrastructure/observability/logging"
	gormrepo "yunxia/internal/infrastructure/persistence/gorm"
	infraRSS "yunxia/internal/infrastructure/rss"
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

	dbCtx, dbCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer dbCancel()
	dbRuntime, err := gormrepo.OpenDatabase(dbCtx, cfg.Database)
	if err != nil {
		log.Fatalf("open postgres database: %v", err)
	}
	defer dbRuntime.Close()
	db := dbRuntime.DB
	rootLogger.Info("database runtime ready",
		slog.String("event", "database.ready"),
		slog.Bool("auto_migrate", cfg.Database.AutoMigrate),
		slog.Int("max_open_conns", cfg.Database.MaxOpenConns),
		slog.Int("max_idle_conns", cfg.Database.MaxIdleConns),
	)

	userRepo := gormrepo.NewUserRepository(db)
	refreshRepo := gormrepo.NewRefreshTokenRepository(db)
	systemConfigRepo := gormrepo.NewSystemConfigRepository(db)
	sourceRepo := gormrepo.NewSourceRepository(db)
	uploadRepo := gormrepo.NewUploadSessionRepository(db)
	taskRepo := gormrepo.NewTaskRepository(db)
	rssRepo := gormrepo.NewRSSRepository(db)
	notificationRepo := gormrepo.NewNotificationRepository(db)
	trashRepo := gormrepo.NewTrashItemRepository(db)
	aclRepo := gormrepo.NewACLRuleRepository(db)
	shareRepo := gormrepo.NewShareRepository(db)
	auditRepo := gormrepo.NewAuditLogRepository(db)
	vfsNodeRepo := gormrepo.NewVFSNodeRepository(db)
	storageObjectRepo := gormrepo.NewStorageObjectRepository(db)
	vfsMountRepo := gormrepo.NewVFSMountRepository(db)
	vfsTagRepo := gormrepo.NewVFSTagRepository(db)
	transactor := gormrepo.NewTransactor(db)

	hasher := security.NewBcryptHasher(cfg.Security.BcryptCost)
	tokenSvc := security.NewJWTTokenService(cfg.JWT.Secret, cfg.JWT.AccessTokenExpire, cfg.JWT.RefreshTokenExpire)
	fileAccessSvc := security.NewFileAccessTokenService(cfg.JWT.Secret)
	auditRecorder := appaudit.NewRecorder(auditRepo, appLog.Component(rootLogger, "audit.recorder"))
	auditQuerySvc := appaudit.NewQueryService(auditRepo)
	downloadSvc := downloader.NewAria2Client(cfg.Aria2.RPCURL, cfg.Aria2.RPCSecret)
	downloadRouter := appsvc.NewDownloaderRouter(downloadSvc)
	var qbitClient *downloader.QBittorrentClient
	if cfg.QBittorrent.Enabled {
		qbitClient = downloader.NewQBittorrentClient(cfg.QBittorrent.APIURL, cfg.QBittorrent.Username, cfg.QBittorrent.Password)
		downloadRouter.Register(appsvc.DownloaderTypeQBittorrent, qbitClient)
		rootLogger.Info("qbittorrent downloader enabled",
			slog.String("event", "qbittorrent.enabled"),
			slog.String("api_url", cfg.QBittorrent.APIURL),
			slog.String("download_dir", cfg.QBittorrent.DownloadDir),
		)
	}
	s3Driver := infraStorage.NewS3Driver(infraStorage.NewS3ClientFactory())
	pikPakDriver := infraStorage.NewPikPakDriver(infraStorage.WithPikPakRuntimeConfigWriter(func(ctx context.Context, source *entity.StorageSource, configJSON string) error {
		if source == nil {
			return nil
		}
		current, err := sourceRepo.FindByID(ctx, source.ID)
		if err != nil {
			return err
		}
		current.ConfigJSON = configJSON
		return sourceRepo.Update(ctx, current)
	}))
	storageDrivers := appsvc.NewStorageDriverRegistry(
		appsvc.DriverBundle{
			Type:        "local",
			DisplayName: "Local",
			Config:      appsvc.NewLocalSourceConfigCodec(),
		},
		appsvc.DriverBundle{
			Type:                   "s3",
			DisplayName:            "S3",
			Config:                 appsvc.NewS3SourceConfigCodec(),
			Probe:                  s3Driver,
			File:                   s3Driver,
			Upload:                 s3Driver,
			Import:                 s3Driver,
			RecursiveStatsFallback: true,
		},
		appsvc.DriverBundle{
			Type:           infraStorage.PikPakDriverType,
			DisplayName:    "PikPak",
			Config:         appsvc.NewPikPakSourceConfigCodec(),
			Probe:          pikPakDriver,
			File:           pikPakDriver,
			Upload:         pikPakDriver,
			Import:         pikPakDriver,
			NativeDownload: pikPakDriver,
			Capacity:       pikPakDriver,
			Capabilities:   pikPakDriver,
		},
	)

	options := appsvc.DefaultSystemOptions()
	options.StorageDataDir = cfg.Storage.DataDir
	options.TempDir = cfg.Storage.TempDir
	options.DefaultChunkSize = cfg.Storage.DefaultChunkSize
	options.MaxUploadSize = cfg.Storage.MaxUploadSize
	options.WebDAVEnabled = cfg.WebDAV.Enabled
	options.WebDAVPrefix = cfg.WebDAV.Prefix

	metadataVFSMountSvc := appsvc.NewMetadataVFSMountService(
		vfsNodeRepo,
		vfsMountRepo,
		sourceRepo,
		appsvc.WithMetadataVFSMountTransactor(transactor),
	)
	setupSvc := appsvc.NewSetupService(
		userRepo,
		refreshRepo,
		systemConfigRepo,
		sourceRepo,
		hasher,
		tokenSvc,
		options,
		appsvc.WithSetupAuditRecorder(auditRecorder),
		appsvc.WithSetupSourceMountSyncer(metadataVFSMountSvc),
	)
	authSvc := appsvc.NewAuthService(userRepo, refreshRepo, hasher, tokenSvc)
	systemServiceOptions := []appsvc.SystemServiceOption{
		appsvc.WithSystemAuditRecorder(auditRecorder),
		appsvc.WithSystemStatsDependencies(userRepo, sourceRepo, taskRepo),
	}
	systemServiceOptions = append(systemServiceOptions, storageDrivers.SystemServiceOptions()...)
	systemSvc := appsvc.NewSystemService(systemConfigRepo, options, systemServiceOptions...)
	aclAuthorizer := appsvc.NewACLAuthorizer(systemConfigRepo, aclRepo, sourceRepo)
	sourceServiceOptions := []appsvc.SourceServiceOption{
		appsvc.WithSourceAuditRecorder(auditRecorder),
		appsvc.WithSourceACLAuthorizer(aclAuthorizer),
	}
	sourceServiceOptions = append(sourceServiceOptions, storageDrivers.SourceServiceOptions()...)
	sourceServiceOptions = append(sourceServiceOptions,
		appsvc.WithSourceMountSyncer(metadataVFSMountSvc),
		appsvc.WithSourceTransactor(transactor),
	)
	sourceSvc := appsvc.NewSourceService(sourceRepo, systemConfigRepo, sourceServiceOptions...)
	if syncResult, syncErr := metadataVFSMountSvc.SyncAllSourceMounts(context.Background()); syncErr != nil {
		rootLogger.Warn("metadata vfs source mount bootstrap failed",
			slog.String("event", "metadata_vfs.mount.bootstrap_failed"),
			slog.Any("error", syncErr),
		)
	} else if syncResult.Failed > 0 {
		rootLogger.Warn("metadata vfs source mount bootstrap completed with failures",
			slog.String("event", "metadata_vfs.mount.bootstrap_partial"),
			slog.Int("synced", syncResult.Synced),
			slog.Int("failed", syncResult.Failed),
			slog.Any("errors", syncResult.Errors),
		)
	} else {
		rootLogger.Info("metadata vfs source mount bootstrap completed",
			slog.String("event", "metadata_vfs.mount.bootstrap_ok"),
			slog.Int("synced", syncResult.Synced),
		)
	}
	userSvc := appsvc.NewUserService(userRepo, hasher, appsvc.WithUserAuditRecorder(auditRecorder))
	aclSvc := appsvc.NewACLService(sourceRepo, userRepo, aclRepo, appsvc.WithACLAuditRecorder(auditRecorder))
	fileServiceOptions := []appsvc.FileServiceOption{
		appsvc.WithFileAuditRecorder(auditRecorder),
		appsvc.WithFileACLAuthorizer(aclAuthorizer),
		appsvc.WithTrashItemRepository(trashRepo),
	}
	fileServiceOptions = append(fileServiceOptions, storageDrivers.FileServiceOptions()...)
	fileSvc := appsvc.NewFileService(sourceRepo, fileAccessSvc, tokenSvc, userRepo, fileServiceOptions...)
	trashServiceOptions := []appsvc.TrashServiceOption{
		appsvc.WithTrashAuditRecorder(auditRecorder),
		appsvc.WithTrashACLAuthorizer(aclAuthorizer),
	}
	trashServiceOptions = append(trashServiceOptions, storageDrivers.TrashServiceOptions()...)
	trashSvc := appsvc.NewTrashService(sourceRepo, trashRepo, trashServiceOptions...)
	metadataVFSReader := appsvc.NewMetadataVFSService(
		vfsNodeRepo,
		appsvc.WithMetadataVFSTransactor(transactor),
	)
	metadataVFSSyncOptions := []appsvc.MetadataVFSSyncServiceOption{
		appsvc.WithMetadataVFSSyncMountRepository(vfsMountRepo),
		appsvc.WithMetadataVFSSyncTransactor(transactor),
	}
	metadataVFSSyncOptions = append(metadataVFSSyncOptions, storageDrivers.MetadataVFSSyncServiceOptions()...)
	metadataVFSSyncSvc := appsvc.NewMetadataVFSSyncService(
		vfsNodeRepo,
		storageObjectRepo,
		sourceRepo,
		metadataVFSSyncOptions...,
	)
	vfsServiceOptions := []appsvc.VFSServiceOption{
		appsvc.WithVFSFileOperator(fileSvc),
		appsvc.WithVFSACLAuthorizer(aclAuthorizer),
		appsvc.WithVFSMetadataServices(metadataVFSReader, metadataVFSSyncSvc),
	}
	vfsServiceOptions = append(vfsServiceOptions, storageDrivers.VFSServiceOptions()...)
	vfsSvc := appsvc.NewVFSService(sourceRepo, vfsServiceOptions...)
	metadataVFSCommitter := appsvc.NewMetadataVFSCommitService(
		vfsNodeRepo,
		storageObjectRepo,
		appsvc.WithMetadataVFSCommitTransactor(transactor),
	)
	uploadServiceOptions := []appsvc.UploadServiceOption{
		appsvc.WithUploadAuditRecorder(auditRecorder),
		appsvc.WithUploadACLAuthorizer(aclAuthorizer),
		appsvc.WithUploadVFSResolver(vfsSvc),
		appsvc.WithUploadMetadataVFSCommitter(metadataVFSCommitter),
	}
	uploadServiceOptions = append(uploadServiceOptions, storageDrivers.UploadServiceOptions()...)
	uploadSvc := appsvc.NewUploadService(sourceRepo, uploadRepo, options, uploadServiceOptions...)
	taskServiceOptions := []appsvc.TaskServiceOption{
		appsvc.WithTaskAuditRecorder(auditRecorder),
		appsvc.WithTaskACLAuthorizer(aclAuthorizer),
		appsvc.WithTaskStagingDir(taskStagingRoot(cfg)),
		appsvc.WithTaskDownloaderStagingDir(appsvc.DownloaderTypeAria2, downloadStagingRoot(cfg.Aria2.DownloadDir)),
		appsvc.WithTaskDownloaderStagingDir(appsvc.DownloaderTypeQBittorrent, downloadStagingRoot(cfg.QBittorrent.DownloadDir)),
		appsvc.WithTaskVFSResolver(vfsSvc),
		appsvc.WithTaskMetadataVFSCommitter(metadataVFSCommitter),
		appsvc.WithTaskDownloadRouter(downloadRouter),
	}
	taskServiceOptions = append(taskServiceOptions, storageDrivers.TaskServiceOptions()...)
	taskSvc := appsvc.NewTaskService(taskRepo, sourceRepo, downloadSvc, taskServiceOptions...)
	notificationSvc := appsvc.NewNotificationService(
		notificationRepo,
		appsvc.WithNotificationWebhookSender(infraNotification.NewWebhookSender(10*time.Second)),
		appsvc.WithNotificationLogger(appLog.Component(rootLogger, "service.notification")),
	)
	go notificationSvc.StartRetryWorker(context.Background(), time.Minute)
	rssOptions := []appsvc.RSSServiceOption{
		appsvc.WithRSSFetcher(infraRSS.NewFetcher()),
		appsvc.WithRSSVFSResolver(vfsSvc),
		appsvc.WithRSSACLAuthorizer(aclAuthorizer),
		appsvc.WithRSSUserRepository(userRepo),
		appsvc.WithRSSTaskRepository(taskRepo),
		appsvc.WithRSSNotifier(notificationSvc),
	}
	if qbitClient != nil {
		rssOptions = append(rssOptions, appsvc.WithRSSQBitHealthChecker(qbitClient))
	}
	rssSvc := appsvc.NewRSSService(
		rssRepo,
		sourceRepo,
		taskSvc,
		rssOptions...,
	)
	taskSvc.SetTerminalStatusObserver(rssSvc)
	go taskSvc.StartSyncWorker(context.Background(), 5*time.Second)
	go rssSvc.StartRefreshWorker(context.Background(), time.Minute)
	go rssSvc.StartRetryWorker(context.Background(), time.Minute)
	shareServiceOptions := []appsvc.ShareServiceOption{
		appsvc.WithShareAuditRecorder(auditRecorder),
		appsvc.WithShareACLAuthorizer(aclAuthorizer),
		appsvc.WithShareMetadataVFS(metadataVFSReader, metadataVFSSyncSvc),
	}
	shareServiceOptions = append(shareServiceOptions, storageDrivers.ShareServiceOptions()...)
	shareSvc := appsvc.NewShareService(shareRepo, sourceRepo, hasher, fileAccessSvc, shareServiceOptions...)
	vfsTagSvc := appsvc.NewVFSTagService(vfsNodeRepo, vfsTagRepo)

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
	notificationHandler := httphandler.NewNotificationHandler(notificationSvc)
	rssHandler := httphandler.NewRSSHandler(rssSvc)
	shareHandler := httphandler.NewShareHandler(shareSvc)
	vfsHandler := httphandler.NewVFSHandler(vfsSvc, fileSvc)
	vfsTagHandler := httphandler.NewVFSTagHandler(vfsTagSvc)
	webdavHandler := httphandler.NewWebDAVHandler(
		cfg.WebDAV.Prefix,
		sourceRepo,
		systemConfigRepo,
		userRepo,
		aclAuthorizer,
		fileSvc,
		uploadSvc,
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
	httpiface.RegisterNotificationRoutes(engine, notificationHandler, authMW, auditRecorder, rootLogger)
	httpiface.RegisterTaskRoutes(engine, taskHandler, authMW)
	httpiface.RegisterRSSRoutes(engine, rssHandler, authMW, auditRecorder, rootLogger)
	httpiface.RegisterShareRoutes(engine, shareHandler, authMW)
	httpiface.RegisterVFSRoutes(engine, vfsHandler, authMW)
	httpiface.RegisterVFSTagRoutes(engine, vfsTagHandler, authMW)
	httpiface.RegisterWebDAVRoutes(engine, cfg.WebDAV.Prefix, webdavHandler)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	rootLogger.Info("yunxia backend listening", slog.String("event", "app.start"), slog.String("addr", addr))
	if err := engine.Run(addr); err != nil {
		rootLogger.Error("run server failed", slog.String("event", "app.stop"), slog.Any("error", err))
		log.Fatalf("run server: %v", err)
	}
}

func prepareDirectories(cfg appcfg.Config) error {
	for _, dir := range []string{cfg.Storage.DataDir, cfg.Storage.TempDir, cfg.Aria2.DownloadDir, cfg.QBittorrent.DownloadDir} {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	return nil
}

func taskStagingRoot(cfg appcfg.Config) string {
	if strings.TrimSpace(cfg.Aria2.DownloadDir) != "" {
		return downloadStagingRoot(cfg.Aria2.DownloadDir)
	}
	if strings.TrimSpace(cfg.QBittorrent.DownloadDir) != "" {
		return downloadStagingRoot(cfg.QBittorrent.DownloadDir)
	}
	return filepath.Join(cfg.Storage.TempDir, "downloads")
}

func downloadStagingRoot(downloadDir string) string {
	if strings.TrimSpace(downloadDir) == "" {
		return ""
	}
	return slashpath.Join(filepath.ToSlash(downloadDir), "staging")
}
