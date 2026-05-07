package service

import (
	"context"
	"errors"
	"strings"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

// VFSTagService 负责 VFS 控制面标签与节点绑定。
type VFSTagService struct {
	nodeRepo domainrepo.VFSNodeRepository
	tagRepo  domainrepo.VFSTagRepository
	now      func() time.Time
}

// VFSTagServiceOption 定义 VFSTagService 可选依赖。
type VFSTagServiceOption func(*VFSTagService)

// WithVFSTagClock 覆盖当前时间，主要用于测试。
func WithVFSTagClock(now func() time.Time) VFSTagServiceOption {
	return func(s *VFSTagService) {
		if now != nil {
			s.now = now
		}
	}
}

// NewVFSTagService 创建 VFS 标签服务。
func NewVFSTagService(nodeRepo domainrepo.VFSNodeRepository, tagRepo domainrepo.VFSTagRepository, options ...VFSTagServiceOption) *VFSTagService {
	service := &VFSTagService{
		nodeRepo: nodeRepo,
		tagRepo:  tagRepo,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// ListTags 列出当前用户拥有的标签。
func (s *VFSTagService) ListTags(ctx context.Context, ownerUserID uint) (*appdto.VFSTagListResponse, error) {
	if err := s.ensureReady(ownerUserID); err != nil {
		return nil, err
	}
	tags, err := s.tagRepo.List(ctx, domainrepo.VFSTagListFilter{OwnerUserID: ownerUserID})
	if err != nil {
		return nil, normalizeMetadataVFSError(err)
	}
	return &appdto.VFSTagListResponse{Items: vfsTagViews(tags)}, nil
}

// CreateTag 创建或更新当前用户同名标签。
func (s *VFSTagService) CreateTag(ctx context.Context, ownerUserID uint, req appdto.VFSTagUpsertRequest) (*appdto.VFSTagView, error) {
	if err := s.ensureReady(ownerUserID); err != nil {
		return nil, err
	}
	name, color, err := normalizeVFSTagInput(req)
	if err != nil {
		return nil, err
	}
	now := s.now()
	tag := &entity.VFSTag{
		OwnerUserID: ownerUserID,
		Name:        name,
		Color:       color,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.tagRepo.UpsertByOwnerAndName(ctx, tag); err != nil {
		return nil, normalizeMetadataVFSError(err)
	}
	view := vfsTagView(tag)
	return &view, nil
}

// UpdateTag 更新当前用户拥有的标签。
func (s *VFSTagService) UpdateTag(ctx context.Context, ownerUserID uint, tagID uint, req appdto.VFSTagUpsertRequest) (*appdto.VFSTagView, error) {
	if err := s.ensureReady(ownerUserID); err != nil {
		return nil, err
	}
	if tagID == 0 {
		return nil, ErrTagInvalid
	}
	name, color, err := normalizeVFSTagInput(req)
	if err != nil {
		return nil, err
	}
	tag, err := s.requireOwnedTag(ctx, ownerUserID, tagID)
	if err != nil {
		return nil, err
	}
	tag.Name = name
	tag.Color = color
	tag.UpdatedAt = s.now()
	if err := s.tagRepo.Update(ctx, tag); err != nil {
		return nil, normalizeMetadataVFSError(err)
	}
	view := vfsTagView(tag)
	return &view, nil
}

// DeleteTag 删除当前用户拥有的标签及其绑定。
func (s *VFSTagService) DeleteTag(ctx context.Context, ownerUserID uint, tagID uint) error {
	if err := s.ensureReady(ownerUserID); err != nil {
		return err
	}
	if tagID == 0 {
		return ErrTagInvalid
	}
	if _, err := s.requireOwnedTag(ctx, ownerUserID, tagID); err != nil {
		return err
	}
	if err := s.tagRepo.Delete(ctx, tagID); err != nil {
		return normalizeMetadataVFSError(err)
	}
	return nil
}

// ListNodeTags 列出指定 VFS 节点上当前用户可见的标签。
func (s *VFSTagService) ListNodeTags(ctx context.Context, ownerUserID uint, virtualPath string) (*appdto.VFSNodeTagListResponse, error) {
	if err := s.ensureReady(ownerUserID); err != nil {
		return nil, err
	}
	node, err := s.resolveNode(ctx, virtualPath)
	if err != nil {
		return nil, err
	}
	tags, err := s.tagRepo.ListTagsForNode(ctx, node.ID)
	if err != nil {
		return nil, normalizeMetadataVFSError(err)
	}
	owned := make([]*entity.VFSTag, 0, len(tags))
	for _, tag := range tags {
		if tag != nil && tag.OwnerUserID == ownerUserID {
			owned = append(owned, tag)
		}
	}
	return &appdto.VFSNodeTagListResponse{
		Path: node.Path,
		Tags: vfsTagViews(owned),
	}, nil
}

// AttachTag 把当前用户拥有的标签绑定到 VFS 节点。
func (s *VFSTagService) AttachTag(ctx context.Context, ownerUserID uint, req appdto.VFSNodeTagRequest) (*appdto.VFSNodeTagListResponse, error) {
	if err := s.ensureReady(ownerUserID); err != nil {
		return nil, err
	}
	if req.TagID == 0 {
		return nil, ErrTagInvalid
	}
	tag, err := s.requireOwnedTag(ctx, ownerUserID, req.TagID)
	if err != nil {
		return nil, err
	}
	node, err := s.resolveNode(ctx, req.Path)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if err := s.tagRepo.AttachToNode(ctx, &entity.VFSNodeTag{
		NodeID:    node.ID,
		TagID:     tag.ID,
		CreatedBy: &ownerUserID,
		CreatedAt: now,
	}); err != nil {
		return nil, normalizeMetadataVFSError(err)
	}
	return s.ListNodeTags(ctx, ownerUserID, node.Path)
}

// DetachTag 解除当前用户拥有的标签与 VFS 节点绑定。
func (s *VFSTagService) DetachTag(ctx context.Context, ownerUserID uint, req appdto.VFSNodeTagRequest) (*appdto.VFSNodeTagListResponse, error) {
	if err := s.ensureReady(ownerUserID); err != nil {
		return nil, err
	}
	if req.TagID == 0 {
		return nil, ErrTagInvalid
	}
	if _, err := s.requireOwnedTag(ctx, ownerUserID, req.TagID); err != nil {
		return nil, err
	}
	node, err := s.resolveNode(ctx, req.Path)
	if err != nil {
		return nil, err
	}
	if err := s.tagRepo.DetachFromNode(ctx, node.ID, req.TagID); err != nil {
		if errors.Is(err, domainrepo.ErrNotFound) {
			return nil, ErrTagBindingNotFound
		}
		return nil, normalizeMetadataVFSError(err)
	}
	return s.ListNodeTags(ctx, ownerUserID, node.Path)
}

func (s *VFSTagService) ensureReady(ownerUserID uint) error {
	if s == nil || s.nodeRepo == nil || s.tagRepo == nil {
		return ErrSourceDriverUnsupported
	}
	if ownerUserID == 0 {
		return ErrPermissionDenied
	}
	return nil
}

func (s *VFSTagService) requireOwnedTag(ctx context.Context, ownerUserID uint, tagID uint) (*entity.VFSTag, error) {
	tag, err := s.tagRepo.FindByID(ctx, tagID)
	if err != nil {
		if errors.Is(err, domainrepo.ErrNotFound) {
			return nil, domainrepo.ErrNotFound
		}
		return nil, normalizeMetadataVFSError(err)
	}
	if tag.OwnerUserID != ownerUserID {
		return nil, ErrTagForbidden
	}
	return tag, nil
}

func (s *VFSTagService) resolveNode(ctx context.Context, virtualPath string) (*entity.VFSNode, error) {
	normalizedPath, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return nil, err
	}
	node, err := s.nodeRepo.FindByPath(ctx, normalizedPath)
	if err != nil {
		return nil, normalizeMetadataVFSError(err)
	}
	return node, nil
}

func normalizeVFSTagInput(req appdto.VFSTagUpsertRequest) (string, string, error) {
	name := strings.TrimSpace(req.Name)
	color := strings.TrimSpace(req.Color)
	if name == "" || len(name) > 64 || strings.ContainsAny(name, "\r\n\t") {
		return "", "", ErrTagInvalid
	}
	if len(color) > 32 || strings.ContainsAny(color, "\r\n\t") {
		return "", "", ErrTagInvalid
	}
	return name, color, nil
}

func vfsTagViews(tags []*entity.VFSTag) []appdto.VFSTagView {
	items := make([]appdto.VFSTagView, 0, len(tags))
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		items = append(items, vfsTagView(tag))
	}
	return items
}

func vfsTagView(tag *entity.VFSTag) appdto.VFSTagView {
	if tag == nil {
		return appdto.VFSTagView{}
	}
	return appdto.VFSTagView{
		ID:        tag.ID,
		Name:      tag.Name,
		Color:     tag.Color,
		CreatedAt: tag.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: tag.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
