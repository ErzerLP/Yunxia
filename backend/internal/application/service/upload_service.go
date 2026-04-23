package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	appaudit "yunxia/internal/application/audit"
	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
	"yunxia/internal/infrastructure/security"
)

// UploadService 负责上传初始化、分片和完成逻辑。
type UploadService struct {
	sourceRepo    domainrepo.SourceRepository
	uploadRepo    domainrepo.UploadSessionRepository
	aclAuthorizer *ACLAuthorizer
	options       SystemOptions
	uploadDrivers map[string]UploadDriver
	vfsResolver   interface {
		ResolveWritableTarget(ctx context.Context, virtualPath string) (ResolvedPath, error)
	}
	logger        *slog.Logger
	auditRecorder *appaudit.Recorder
}

type resolvedUploadInitTarget struct {
	source                  *entity.StorageSource
	path                    string
	targetVirtualParentPath string
	resolvedSourceID        uint
	resolvedInnerParentPath string
}

// NewUploadService 创建上传服务。
func NewUploadService(sourceRepo domainrepo.SourceRepository, uploadRepo domainrepo.UploadSessionRepository, options SystemOptions, serviceOptions ...UploadServiceOption) *UploadService {
	service := &UploadService{
		sourceRepo:    sourceRepo,
		uploadRepo:    uploadRepo,
		options:       options,
		uploadDrivers: make(map[string]UploadDriver),
		logger:        newServiceLogger("service.upload"),
	}
	for _, option := range serviceOptions {
		option(service)
	}
	return service
}

// Init 初始化上传会话。
func (s *UploadService) Init(ctx context.Context, userID uint, req appdto.UploadInitRequest) (*appdto.UploadInitResponse, error) {
	if err := validateFileName(req.Filename); err != nil {
		return nil, err
	}

	target, err := s.resolveInitTarget(ctx, req)
	if err != nil {
		return nil, err
	}
	req.SourceID = target.source.ID
	req.Path = target.path

	if err := s.authorizePath(ctx, target.source.ID, target.path, ACLActionWrite); err != nil {
		return nil, err
	}
	if target.source.DriverType != "local" {
		return s.initWithUploadDriver(ctx, userID, target, req)
	}
	return s.initLocal(ctx, userID, target, req)
}

func (s *UploadService) initLocal(ctx context.Context, userID uint, target resolvedUploadInitTarget, req appdto.UploadInitRequest) (*appdto.UploadInitResponse, error) {
	if req.FileSize > s.options.MaxUploadSize {
		return nil, ErrUploadTooLarge
	}
	targetDir := target.path

	targetVirtual := path.Join(targetDir, req.Filename)
	if targetDir == "/" {
		targetVirtual = "/" + req.Filename
	}
	_, targetPhysical, err := resolvePhysicalPath(target.source, targetVirtual)
	if err != nil {
		return nil, err
	}
	if _, statErr := os.Stat(targetPhysical); statErr == nil {
		hash, hashErr := hashFileMD5(targetPhysical)
		if hashErr == nil && hash == req.FileHash {
			info, _ := os.Stat(targetPhysical)
			item := buildFileItem(target.source.ID, targetVirtual, info)
			return &appdto.UploadInitResponse{
				IsFastUpload:     true,
				File:             &item,
				PartInstructions: []appdto.UploadPartInstruction{},
			}, nil
		}
		return nil, ErrFileAlreadyExists
	}

	existing, err := s.uploadRepo.FindActiveByIdentity(ctx, userID, req.SourceID, targetDir, req.Filename, req.FileSize, req.FileHash)
	if err == nil {
		session := toUploadSessionView(existing)
		return &appdto.UploadInitResponse{
			IsFastUpload: false,
			Upload:       &session,
			Transport: &appdto.UploadTransport{
				Mode:        "server_chunk",
				DriverType:  "local",
				Concurrency: 3,
				RetryLimit:  3,
			},
			PartInstructions: []appdto.UploadPartInstruction{},
		}, nil
	}
	if err != nil && !errors.Is(err, domainrepo.ErrNotFound) {
		return nil, err
	}

	totalChunks := int((req.FileSize + s.options.DefaultChunkSize - 1) / s.options.DefaultChunkSize)
	if totalChunks <= 0 {
		totalChunks = 1
	}
	now := time.Now()
	session := &entity.UploadSession{
		UploadID:                "upl_" + stringsNoDash(uuid.NewString()),
		UserID:                  userID,
		SourceID:                req.SourceID,
		Path:                    targetDir,
		TargetVirtualParentPath: target.targetVirtualParentPath,
		ResolvedSourceID:        target.resolvedSourceID,
		ResolvedInnerParentPath: target.resolvedInnerParentPath,
		Filename:                req.Filename,
		FileSize:                req.FileSize,
		FileHash:                req.FileHash,
		ChunkSize:               s.options.DefaultChunkSize,
		TotalChunks:             totalChunks,
		UploadedChunks:          []int{},
		Status:                  "uploading",
		IsFastUpload:            false,
		ExpiresAt:               now.Add(7 * 24 * time.Hour),
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := s.uploadRepo.Create(ctx, session); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.sessionTempDir(session.UploadID), 0o755); err != nil {
		return nil, err
	}

	view := toUploadSessionView(session)
	return &appdto.UploadInitResponse{
		IsFastUpload: false,
		Upload:       &view,
		Transport: &appdto.UploadTransport{
			Mode:        "server_chunk",
			DriverType:  "local",
			Concurrency: 3,
			RetryLimit:  3,
		},
		PartInstructions: []appdto.UploadPartInstruction{},
	}, nil
}

func (s *UploadService) initWithUploadDriver(ctx context.Context, userID uint, target resolvedUploadInitTarget, req appdto.UploadInitRequest) (*appdto.UploadInitResponse, error) {
	driver, err := s.getUploadDriver(target.source.DriverType)
	if err != nil {
		return nil, err
	}
	if req.FileSize > s.options.MaxUploadSize {
		return nil, ErrUploadTooLarge
	}
	targetDir := target.path

	partSize := s.options.DefaultChunkSize
	totalChunks := int((req.FileSize + partSize - 1) / partSize)
	if totalChunks <= 0 {
		totalChunks = 1
	}

	plan, err := driver.InitMultipartUpload(ctx, target.source, MultipartUploadRequest{
		VirtualPath: targetDir,
		Filename:    req.Filename,
		ContentType: detectContentType(req.Filename),
		FileSize:    req.FileSize,
		PartSize:    partSize,
		TotalParts:  totalChunks,
		ExpiresIn:   15 * time.Minute,
	})
	if err != nil {
		return nil, err
	}
	stateJSON, err := json.Marshal(plan.State)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	session := &entity.UploadSession{
		UploadID:                "upl_" + stringsNoDash(uuid.NewString()),
		UserID:                  userID,
		SourceID:                req.SourceID,
		Path:                    targetDir,
		TargetVirtualParentPath: target.targetVirtualParentPath,
		ResolvedSourceID:        target.resolvedSourceID,
		ResolvedInnerParentPath: target.resolvedInnerParentPath,
		Filename:                req.Filename,
		FileSize:                req.FileSize,
		FileHash:                req.FileHash,
		ChunkSize:               partSize,
		TotalChunks:             totalChunks,
		UploadedChunks:          []int{},
		StorageDataJSON:         string(stateJSON),
		Status:                  "uploading",
		IsFastUpload:            false,
		ExpiresAt:               now.Add(7 * 24 * time.Hour),
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := s.uploadRepo.Create(ctx, session); err != nil {
		return nil, err
	}

	view := toUploadSessionView(session)
	return &appdto.UploadInitResponse{
		IsFastUpload: false,
		Upload:       &view,
		Transport: &appdto.UploadTransport{
			Mode:        "direct_parts",
			DriverType:  target.source.DriverType,
			Concurrency: 3,
			RetryLimit:  3,
		},
		PartInstructions: toUploadPartInstructions(plan.PartInstructions),
	}, nil
}

// UploadChunk 接收单个 chunk。
func (s *UploadService) UploadChunk(ctx context.Context, uploadID string, index int, data []byte) (*appdto.UploadChunkResponse, error) {
	session, err := s.uploadRepo.FindByID(ctx, uploadID)
	if err != nil {
		if errors.Is(err, domainrepo.ErrNotFound) {
			return nil, ErrUploadSessionNotFound
		}
		return nil, err
	}
	if err := s.authorizeUploadSession(ctx, session); err != nil {
		return nil, err
	}
	if index < 0 || index >= session.TotalChunks {
		return nil, ErrUploadInvalidState
	}
	chunkPath := s.chunkFilePath(uploadID, index)
	if existing, readErr := os.ReadFile(chunkPath); readErr == nil {
		if slices.Equal(existing, data) {
			return &appdto.UploadChunkResponse{
				UploadID:        uploadID,
				Index:           index,
				ReceivedBytes:   int64(len(data)),
				AlreadyUploaded: true,
			}, nil
		}
		return nil, ErrUploadChunkConflict
	}

	if err := os.MkdirAll(filepath.Dir(chunkPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(chunkPath, data, 0o644); err != nil {
		return nil, err
	}

	if !slices.Contains(session.UploadedChunks, index) {
		session.UploadedChunks = append(session.UploadedChunks, index)
		slices.Sort(session.UploadedChunks)
	}
	session.UpdatedAt = time.Now()
	if err := s.uploadRepo.Update(ctx, session); err != nil {
		return nil, err
	}

	return &appdto.UploadChunkResponse{
		UploadID:        uploadID,
		Index:           index,
		ReceivedBytes:   int64(len(data)),
		AlreadyUploaded: false,
	}, nil
}

// Finish 完成上传并合并文件。
func (s *UploadService) Finish(ctx context.Context, req appdto.UploadFinishRequest) (*appdto.UploadFinishResponse, error) {
	session, err := s.uploadRepo.FindByID(ctx, req.UploadID)
	if err != nil {
		if errors.Is(err, domainrepo.ErrNotFound) {
			recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
				ResourceType: "file",
				Action:       "upload_finish",
				Result:       appaudit.ResultFailed,
				ErrorCode:    "UPLOAD_SESSION_NOT_FOUND",
				ResourceID:   req.UploadID,
			})
			return nil, ErrUploadSessionNotFound
		}
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "file",
			Action:       "upload_finish",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
			ResourceID:   req.UploadID,
		})
		return nil, err
	}
	if err := s.authorizeUploadSession(ctx, session); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "file",
			Action:       "upload_finish",
			Result:       appaudit.ResultDenied,
			ErrorCode:    "PERMISSION_DENIED",
			ResourceID:   req.UploadID,
			SourceID:     &session.SourceID,
			VirtualPath:  uploadSessionTargetVirtualPath(session),
		})
		return nil, err
	}
	source, err := s.sourceRepo.FindByID(ctx, session.SourceID)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "file",
			Action:       "upload_finish",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "SOURCE_NOT_FOUND",
			ResourceID:   req.UploadID,
			SourceID:     &session.SourceID,
			VirtualPath:  uploadSessionTargetVirtualPath(session),
		})
		return nil, err
	}
	if source.DriverType != "local" {
		resp, err := s.finishWithUploadDriver(ctx, source, session, req)
		if err != nil {
			recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
				ResourceType: "file",
				Action:       "upload_finish",
				Result:       appaudit.ResultFailed,
				ErrorCode:    mapFileErrorCode(err),
				ResourceID:   req.UploadID,
				SourceID:     &source.ID,
				VirtualPath:  mergeMountAndInnerPath(source.MountPath, respOrSessionPath(resp, session)),
			})
			return nil, err
		}
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "file",
			Action:       "upload_finish",
			Result:       appaudit.ResultSuccess,
			ResourceID:   req.UploadID,
			SourceID:     &source.ID,
			VirtualPath:  mergeMountAndInnerPath(source.MountPath, resp.File.Path),
			After: map[string]any{
				"virtual_path": mergeMountAndInnerPath(source.MountPath, resp.File.Path),
				"name":         resp.File.Name,
				"size":         resp.File.Size,
			},
		})
		return resp, nil
	}
	resp, err := s.finishLocal(ctx, source, session)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "file",
			Action:       "upload_finish",
			Result:       appaudit.ResultFailed,
			ErrorCode:    mapFileErrorCode(err),
			ResourceID:   req.UploadID,
			SourceID:     &source.ID,
			VirtualPath:  mergeMountAndInnerPath(source.MountPath, joinVirtualPath(session.Path, session.Filename)),
		})
		return nil, err
	}
	recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
		ResourceType: "file",
		Action:       "upload_finish",
		Result:       appaudit.ResultSuccess,
		ResourceID:   req.UploadID,
		SourceID:     &source.ID,
		VirtualPath:  mergeMountAndInnerPath(source.MountPath, resp.File.Path),
		After: map[string]any{
			"virtual_path": mergeMountAndInnerPath(source.MountPath, resp.File.Path),
			"name":         resp.File.Name,
			"size":         resp.File.Size,
		},
	})
	return resp, nil
}

func (s *UploadService) finishLocal(ctx context.Context, source *entity.StorageSource, session *entity.UploadSession) (*appdto.UploadFinishResponse, error) {
	if len(session.UploadedChunks) < session.TotalChunks {
		return nil, ErrUploadFinishIncomplete
	}
	targetVirtual := path.Join(session.Path, session.Filename)
	if session.Path == "/" {
		targetVirtual = "/" + session.Filename
	}
	_, targetPhysical, err := resolvePhysicalPath(source, targetVirtual)
	if err != nil {
		return nil, err
	}
	if _, statErr := os.Stat(targetPhysical); statErr == nil {
		return nil, ErrFileAlreadyExists
	}
	if err := os.MkdirAll(filepath.Dir(targetPhysical), 0o755); err != nil {
		return nil, err
	}

	output, err := os.Create(targetPhysical)
	if err != nil {
		return nil, err
	}
	defer output.Close()

	for index := 0; index < session.TotalChunks; index++ {
		chunkPath := s.chunkFilePath(session.UploadID, index)
		data, readErr := os.ReadFile(chunkPath)
		if readErr != nil {
			return nil, ErrUploadFinishIncomplete
		}
		if _, writeErr := output.Write(data); writeErr != nil {
			return nil, writeErr
		}
	}
	if err := output.Close(); err != nil {
		return nil, err
	}

	hash, err := hashFileMD5(targetPhysical)
	if err != nil {
		return nil, err
	}
	if session.FileHash != "" && hash != session.FileHash {
		return nil, ErrUploadHashMismatch
	}

	info, err := os.Stat(targetPhysical)
	if err != nil {
		return nil, err
	}
	item := buildFileItem(source.ID, targetVirtual, info)

	_ = os.RemoveAll(s.sessionTempDir(session.UploadID))
	_ = s.uploadRepo.Delete(ctx, session.UploadID)

	return &appdto.UploadFinishResponse{
		Completed: true,
		UploadID:  session.UploadID,
		File:      item,
	}, nil
}

func (s *UploadService) finishWithUploadDriver(ctx context.Context, source *entity.StorageSource, session *entity.UploadSession, req appdto.UploadFinishRequest) (*appdto.UploadFinishResponse, error) {
	if len(req.Parts) < session.TotalChunks {
		return nil, ErrUploadFinishIncomplete
	}
	driver, err := s.getUploadDriver(source.DriverType)
	if err != nil {
		return nil, err
	}
	var state MultipartUploadState
	if err := json.Unmarshal([]byte(session.StorageDataJSON), &state); err != nil {
		return nil, ErrUploadInvalidState
	}
	completedParts := make([]CompletedUploadPart, 0, len(req.Parts))
	for _, part := range req.Parts {
		completedParts = append(completedParts, CompletedUploadPart{
			Index: part.Index,
			ETag:  part.ETag,
		})
	}
	entry, err := driver.CompleteMultipartUpload(ctx, source, state, completedParts)
	if err != nil {
		return nil, err
	}
	item := buildStorageEntryItem(source.ID, *entry)

	_ = s.uploadRepo.Delete(ctx, session.UploadID)
	return &appdto.UploadFinishResponse{
		Completed: true,
		UploadID:  session.UploadID,
		File:      item,
	}, nil
}

// ListSessions 返回用户上传会话。
func (s *UploadService) ListSessions(ctx context.Context, userID uint, sourceID *uint, status string) (*appdto.UploadSessionListResponse, error) {
	items, err := s.uploadRepo.ListByUser(ctx, userID, sourceID, status)
	if err != nil {
		return nil, err
	}

	result := make([]appdto.UploadSessionView, 0, len(items))
	for _, item := range items {
		result = append(result, toUploadSessionView(item))
	}
	return &appdto.UploadSessionListResponse{Items: result}, nil
}

// Cancel 删除上传会话与临时文件。
func (s *UploadService) Cancel(ctx context.Context, uploadID string) error {
	session, err := s.uploadRepo.FindByID(ctx, uploadID)
	if err != nil {
		if errors.Is(err, domainrepo.ErrNotFound) {
			return ErrUploadSessionNotFound
		}
		return err
	}
	if err := s.authorizeUploadSession(ctx, session); err != nil {
		return err
	}
	if session.Status == "completed" {
		return ErrUploadInvalidState
	}
	if err := s.uploadRepo.Delete(ctx, uploadID); err != nil {
		return err
	}
	_ = os.RemoveAll(s.sessionTempDir(uploadID))
	return nil
}

func (s *UploadService) sessionTempDir(uploadID string) string {
	return filepath.Join(s.options.TempDir, uploadID)
}

func (s *UploadService) chunkFilePath(uploadID string, index int) string {
	return filepath.Join(s.sessionTempDir(uploadID), fmt.Sprintf("chunk-%06d.part", index))
}

func uploadSessionTargetVirtualPath(session *entity.UploadSession) string {
	if session == nil {
		return ""
	}
	return joinVirtualPath(session.Path, session.Filename)
}

func respOrSessionPath(resp *appdto.UploadFinishResponse, session *entity.UploadSession) string {
	if resp != nil && resp.File.Path != "" {
		return resp.File.Path
	}
	return uploadSessionTargetVirtualPath(session)
}

func toUploadSessionView(session *entity.UploadSession) appdto.UploadSessionView {
	return appdto.UploadSessionView{
		UploadID:                session.UploadID,
		SourceID:                session.SourceID,
		Path:                    session.Path,
		Filename:                session.Filename,
		FileSize:                session.FileSize,
		FileHash:                session.FileHash,
		ChunkSize:               session.ChunkSize,
		TotalChunks:             session.TotalChunks,
		UploadedChunks:          session.UploadedChunks,
		Status:                  session.Status,
		IsFastUpload:            session.IsFastUpload,
		ExpiresAt:               session.ExpiresAt.Format(time.RFC3339),
		TargetVirtualParentPath: session.TargetVirtualParentPath,
		ResolvedSourceID:        session.ResolvedSourceID,
		ResolvedInnerParentPath: session.ResolvedInnerParentPath,
	}
}

func (s *UploadService) resolveInitTarget(ctx context.Context, req appdto.UploadInitRequest) (resolvedUploadInitTarget, error) {
	if strings.TrimSpace(req.TargetVirtualParentPath) != "" {
		virtualParentPath, err := normalizeVirtualPath(req.TargetVirtualParentPath)
		if err != nil {
			return resolvedUploadInitTarget{}, err
		}

		resolver := s.requireUploadVFSResolver()
		resolved, err := resolver.ResolveWritableTarget(ctx, joinVirtualPath(virtualParentPath, req.Filename))
		if err != nil {
			return resolvedUploadInitTarget{}, err
		}
		resolvedInnerParentPath, _, err := splitParentName(resolved.InnerPath)
		if err != nil {
			return resolvedUploadInitTarget{}, err
		}
		return resolvedUploadInitTarget{
			source:                  resolved.Source,
			path:                    resolvedInnerParentPath,
			targetVirtualParentPath: virtualParentPath,
			resolvedSourceID:        resolved.Source.ID,
			resolvedInnerParentPath: resolvedInnerParentPath,
		}, nil
	}

	if req.SourceID == 0 {
		return resolvedUploadInitTarget{}, ErrPathInvalid
	}
	if strings.TrimSpace(req.Path) == "" {
		return resolvedUploadInitTarget{}, ErrPathInvalid
	}

	source, err := s.sourceRepo.FindByID(ctx, req.SourceID)
	if err != nil {
		return resolvedUploadInitTarget{}, err
	}
	normalizedPath, err := normalizeVirtualPath(req.Path)
	if err != nil {
		return resolvedUploadInitTarget{}, err
	}
	return resolvedUploadInitTarget{
		source:                  source,
		path:                    normalizedPath,
		resolvedSourceID:        source.ID,
		resolvedInnerParentPath: normalizedPath,
	}, nil
}

func (s *UploadService) requireUploadVFSResolver() interface {
	ResolveWritableTarget(ctx context.Context, virtualPath string) (ResolvedPath, error)
} {
	if s.vfsResolver == nil {
		return unsupportedUploadVFSResolver{}
	}
	return s.vfsResolver
}

type unsupportedUploadVFSResolver struct{}

func (unsupportedUploadVFSResolver) ResolveWritableTarget(context.Context, string) (ResolvedPath, error) {
	return ResolvedPath{}, ErrSourceDriverUnsupported
}

func stringsNoDash(value string) string {
	return strings.ReplaceAll(value, "-", "")
}

func (s *UploadService) getUploadDriver(driverType string) (UploadDriver, error) {
	driver, exists := s.uploadDrivers[driverType]
	if !exists {
		return nil, ErrSourceDriverUnsupported
	}
	return driver, nil
}

func toUploadPartInstructions(items []MultipartUploadPartInstruction) []appdto.UploadPartInstruction {
	result := make([]appdto.UploadPartInstruction, 0, len(items))
	for _, item := range items {
		instruction := appdto.UploadPartInstruction{
			Index:     item.Index,
			Method:    item.Method,
			URL:       item.URL,
			Headers:   item.Headers,
			ExpiresAt: item.ExpiresAt.Format(time.RFC3339),
		}
		instruction.ByteRange.Start = item.ByteStart
		instruction.ByteRange.End = item.ByteEnd
		result = append(result, instruction)
	}
	return result
}

func detectContentType(filename string) string {
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
	if contentType == "" {
		return "application/octet-stream"
	}
	return contentType
}

func (s *UploadService) authorizePath(ctx context.Context, sourceID uint, pathValue string, action ACLAction) error {
	if s.aclAuthorizer == nil {
		return nil
	}
	return s.aclAuthorizer.AuthorizePath(ctx, sourceID, pathValue, action)
}

func (s *UploadService) authorizeUploadSession(ctx context.Context, session *entity.UploadSession) error {
	auth, ok := security.RequestAuthFromContext(ctx)
	if !ok || auth.UserID != session.UserID {
		return ErrPermissionDenied
	}
	return s.authorizePath(ctx, session.SourceID, session.Path, ACLActionWrite)
}
