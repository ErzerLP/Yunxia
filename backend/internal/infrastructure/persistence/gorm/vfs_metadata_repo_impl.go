package gorm

import (
	"context"
	"strings"
	"time"

	gormpkg "gorm.io/gorm"
	"gorm.io/gorm/clause"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

var (
	_ domainrepo.VFSNodeRepository       = (*VFSNodeRepository)(nil)
	_ domainrepo.StorageObjectRepository = (*StorageObjectRepository)(nil)
	_ domainrepo.VFSMountRepository      = (*VFSMountRepository)(nil)
	_ domainrepo.VFSTagRepository        = (*VFSTagRepository)(nil)
)

// VFSNodeRepository 提供 VFS 控制面节点仓储实现。
type VFSNodeRepository struct {
	db *gormpkg.DB
}

// NewVFSNodeRepository 创建 VFS node 仓储。
func NewVFSNodeRepository(db *gormpkg.DB) *VFSNodeRepository {
	return &VFSNodeRepository{db: db}
}

// Create 创建 VFS node。
func (r *VFSNodeRepository) Create(ctx context.Context, node *entity.VFSNode) error {
	model := vfsNodeModelFromEntity(node)
	if err := dbFor(ctx, r.db).Create(model).Error; err != nil {
		return normalizeGormError(err)
	}
	*node = *vfsNodeEntityFromModel(model)
	return nil
}

// Update 更新 VFS node。
func (r *VFSNodeRepository) Update(ctx context.Context, node *entity.VFSNode) error {
	model := vfsNodeModelFromEntity(node)
	result := dbFor(ctx, r.db).
		Model(&VFSNodeModel{}).
		Where("id = ?", node.ID).
		Select("*").
		Omit("ID", "CreatedAt").
		Updates(model)
	if result.Error != nil {
		return normalizeGormError(result.Error)
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}
	return nil
}

// Delete 软删除 VFS node 及其 path 子树。
func (r *VFSNodeRepository) Delete(ctx context.Context, id uint) error {
	return dbFor(ctx, r.db).Transaction(func(tx *gormpkg.DB) error {
		var model VFSNodeModel
		if err := tx.Where("id = ? AND is_deleted = ?", id, false).First(&model).Error; err != nil {
			return normalizeGormNotFound(err)
		}

		query := tx.Model(&VFSNodeModel{}).Where("is_deleted = ?", false)
		if model.Path == "/" {
			query = query.Where(`path = ? OR path LIKE ? ESCAPE '\'`, "/", "/%")
		} else {
			query = query.Where(`path = ? OR path LIKE ? ESCAPE '\'`, model.Path, pathChildrenLikePattern(model.Path))
		}

		result := query.Updates(map[string]any{
			"is_deleted": true,
			"updated_at": time.Now().UTC(),
		})
		if result.Error != nil {
			return normalizeGormError(result.Error)
		}
		if result.RowsAffected == 0 {
			return domainrepo.ErrNotFound
		}
		return nil
	})
}

// FindByID 按 ID 查询 VFS node。
func (r *VFSNodeRepository) FindByID(ctx context.Context, id uint) (*entity.VFSNode, error) {
	var model VFSNodeModel
	if err := dbFor(ctx, r.db).First(&model, id).Error; err != nil {
		return nil, normalizeGormNotFound(err)
	}
	return vfsNodeEntityFromModel(&model), nil
}

// FindByPath 按未删除的绝对路径查询 VFS node。
func (r *VFSNodeRepository) FindByPath(ctx context.Context, path string) (*entity.VFSNode, error) {
	var model VFSNodeModel
	if err := dbFor(ctx, r.db).
		Where("path = ? AND is_deleted = ?", path, false).
		First(&model).Error; err != nil {
		return nil, normalizeGormNotFound(err)
	}
	return vfsNodeEntityFromModel(&model), nil
}

// ListChildren 按 parent_id 列出子节点。
func (r *VFSNodeRepository) ListChildren(ctx context.Context, parentID *uint, filter domainrepo.VFSNodeListFilter) ([]*entity.VFSNode, error) {
	query := dbFor(ctx, r.db).Model(&VFSNodeModel{})
	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}
	query = applyVFSNodeListFilter(query, filter)

	var models []VFSNodeModel
	if err := query.Order("name asc, id asc").Find(&models).Error; err != nil {
		return nil, normalizeGormError(err)
	}
	return vfsNodeEntitiesFromModels(models), nil
}

// ListByPathPrefix 按 path 前缀列出节点。
func (r *VFSNodeRepository) ListByPathPrefix(ctx context.Context, pathPrefix string, filter domainrepo.VFSNodeListFilter) ([]*entity.VFSNode, error) {
	query := dbFor(ctx, r.db).Model(&VFSNodeModel{})
	prefix := normalizeVFSPathPrefix(pathPrefix)
	if prefix != "/" {
		query = query.Where(`path = ? OR path LIKE ? ESCAPE '\'`, prefix, pathChildrenLikePattern(prefix))
	}
	query = applyVFSNodeListFilter(query, filter)

	var models []VFSNodeModel
	if err := query.Order("path asc, id asc").Find(&models).Error; err != nil {
		return nil, normalizeGormError(err)
	}
	return vfsNodeEntitiesFromModels(models), nil
}

// UpsertByPath 基于未删除节点的 path 唯一索引创建或更新 VFS node。
func (r *VFSNodeRepository) UpsertByPath(ctx context.Context, node *entity.VFSNode) error {
	node.IsDeleted = false
	model := vfsNodeModelFromEntity(node)
	if err := dbFor(ctx, r.db).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "path"}},
			TargetWhere: clause.Where{Exprs: []clause.Expression{
				clause.Eq{Column: clause.Column{Name: "is_deleted"}, Value: false},
			}},
			DoUpdates: clause.AssignmentColumns([]string{
				"parent_id", "name", "kind", "mount_id", "object_id", "source_id",
				"provider_item_id", "provider_parent_id", "size", "mime_type",
				"etag", "checksum", "sync_state", "is_deleted", "updated_by",
				"updated_at", "indexed_at", "last_seen_at",
			}),
		}).
		Create(model).Error; err != nil {
		return normalizeGormError(err)
	}

	saved, err := r.FindByPath(ctx, node.Path)
	if err != nil {
		return err
	}
	*node = *saved
	return nil
}

// StorageObjectRepository 提供数据面对象仓储实现。
type StorageObjectRepository struct {
	db *gormpkg.DB
}

// NewStorageObjectRepository 创建 storage object 仓储。
func NewStorageObjectRepository(db *gormpkg.DB) *StorageObjectRepository {
	return &StorageObjectRepository{db: db}
}

// Create 创建 storage object。
func (r *StorageObjectRepository) Create(ctx context.Context, object *entity.StorageObject) error {
	model := storageObjectModelFromEntity(object)
	if err := dbFor(ctx, r.db).Create(model).Error; err != nil {
		return normalizeGormError(err)
	}
	*object = *storageObjectEntityFromModel(model)
	return nil
}

// Update 更新 storage object。
func (r *StorageObjectRepository) Update(ctx context.Context, object *entity.StorageObject) error {
	model := storageObjectModelFromEntity(object)
	result := dbFor(ctx, r.db).
		Model(&StorageObjectModel{}).
		Where("id = ?", object.ID).
		Select("*").
		Omit("ID", "CreatedAt").
		Updates(model)
	if result.Error != nil {
		return normalizeGormError(result.Error)
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}
	return nil
}

// Delete 删除 storage object。
func (r *StorageObjectRepository) Delete(ctx context.Context, id uint) error {
	result := dbFor(ctx, r.db).Delete(&StorageObjectModel{}, id)
	if result.Error != nil {
		return normalizeGormError(result.Error)
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}
	return nil
}

// FindByID 按 ID 查询 storage object。
func (r *StorageObjectRepository) FindByID(ctx context.Context, id uint) (*entity.StorageObject, error) {
	var model StorageObjectModel
	if err := dbFor(ctx, r.db).First(&model, id).Error; err != nil {
		return nil, normalizeGormNotFound(err)
	}
	return storageObjectEntityFromModel(&model), nil
}

// FindByLocator 按 source/driver/locator 查询 storage object。
func (r *StorageObjectRepository) FindByLocator(ctx context.Context, sourceID uint, driverType string, locatorType string, locatorJSON string) (*entity.StorageObject, error) {
	var model StorageObjectModel
	if err := dbFor(ctx, r.db).
		Where(
			"source_id = ? AND driver_type = ? AND locator_type = ? AND locator_json = ?::jsonb",
			sourceID,
			driverType,
			locatorType,
			jsonObject(locatorJSON),
		).
		First(&model).Error; err != nil {
		return nil, normalizeGormNotFound(err)
	}
	return storageObjectEntityFromModel(&model), nil
}

// List 按条件列出 storage objects。
func (r *StorageObjectRepository) List(ctx context.Context, filter domainrepo.StorageObjectListFilter) ([]*entity.StorageObject, error) {
	query := dbFor(ctx, r.db).Model(&StorageObjectModel{})
	if filter.SourceID != 0 {
		query = query.Where("source_id = ?", filter.SourceID)
	}
	if filter.DriverType != "" {
		query = query.Where("driver_type = ?", filter.DriverType)
	}
	if filter.LocatorType != "" {
		query = query.Where("locator_type = ?", filter.LocatorType)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}

	var models []StorageObjectModel
	if err := query.Order("created_at desc, id desc").Find(&models).Error; err != nil {
		return nil, normalizeGormError(err)
	}
	items := make([]*entity.StorageObject, 0, len(models))
	for i := range models {
		items = append(items, storageObjectEntityFromModel(&models[i]))
	}
	return items, nil
}

// UpsertByLocator 基于 source/driver/locator 幂等创建或更新 storage object。
func (r *StorageObjectRepository) UpsertByLocator(ctx context.Context, object *entity.StorageObject) error {
	object.LocatorJSON = jsonObject(object.LocatorJSON)
	model := storageObjectModelFromEntity(object)
	if err := dbFor(ctx, r.db).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "source_id"},
				{Name: "driver_type"},
				{Name: "locator_type"},
				{Name: "locator_json"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"size", "etag", "checksum", "mime_type", "status", "updated_at",
			}),
		}).
		Create(model).Error; err != nil {
		return normalizeGormError(err)
	}

	saved, err := r.FindByLocator(ctx, object.SourceID, object.DriverType, object.LocatorType, object.LocatorJSON)
	if err != nil {
		return err
	}
	*object = *saved
	return nil
}

// VFSMountRepository 提供 VFS mount 仓储实现。
type VFSMountRepository struct {
	db *gormpkg.DB
}

// NewVFSMountRepository 创建 VFS mount 仓储。
func NewVFSMountRepository(db *gormpkg.DB) *VFSMountRepository {
	return &VFSMountRepository{db: db}
}

// Create 创建 VFS mount。
func (r *VFSMountRepository) Create(ctx context.Context, mount *entity.VFSMount) error {
	model := vfsMountModelFromEntity(mount)
	if err := dbFor(ctx, r.db).Create(model).Error; err != nil {
		return normalizeGormError(err)
	}
	*mount = *vfsMountEntityFromModel(model)
	return nil
}

// Update 更新 VFS mount。
func (r *VFSMountRepository) Update(ctx context.Context, mount *entity.VFSMount) error {
	model := vfsMountModelFromEntity(mount)
	result := dbFor(ctx, r.db).
		Model(&VFSMountModel{}).
		Where("id = ?", mount.ID).
		Select("*").
		Omit("ID", "CreatedAt").
		Updates(model)
	if result.Error != nil {
		return normalizeGormError(result.Error)
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}
	return nil
}

// Delete 删除 VFS mount。
func (r *VFSMountRepository) Delete(ctx context.Context, id uint) error {
	result := dbFor(ctx, r.db).Delete(&VFSMountModel{}, id)
	if result.Error != nil {
		return normalizeGormError(result.Error)
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}
	return nil
}

// FindByID 按 ID 查询 VFS mount。
func (r *VFSMountRepository) FindByID(ctx context.Context, id uint) (*entity.VFSMount, error) {
	var model VFSMountModel
	if err := dbFor(ctx, r.db).First(&model, id).Error; err != nil {
		return nil, normalizeGormNotFound(err)
	}
	return vfsMountEntityFromModel(&model), nil
}

// FindByNodeID 按挂载节点查询 VFS mount。
func (r *VFSMountRepository) FindByNodeID(ctx context.Context, nodeID uint) (*entity.VFSMount, error) {
	var model VFSMountModel
	if err := dbFor(ctx, r.db).Where("node_id = ?", nodeID).First(&model).Error; err != nil {
		return nil, normalizeGormNotFound(err)
	}
	return vfsMountEntityFromModel(&model), nil
}

// FindByMountPath 按挂载路径查询 VFS mount。
func (r *VFSMountRepository) FindByMountPath(ctx context.Context, mountPath string) (*entity.VFSMount, error) {
	var model VFSMountModel
	if err := dbFor(ctx, r.db).Where("mount_path = ?", mountPath).First(&model).Error; err != nil {
		return nil, normalizeGormNotFound(err)
	}
	return vfsMountEntityFromModel(&model), nil
}

// List 按条件列出 VFS mounts。
func (r *VFSMountRepository) List(ctx context.Context, filter domainrepo.VFSMountListFilter) ([]*entity.VFSMount, error) {
	query := dbFor(ctx, r.db).Model(&VFSMountModel{})
	if filter.SourceID != 0 {
		query = query.Where("source_id = ?", filter.SourceID)
	}
	if filter.Enabled != nil {
		query = query.Where("is_enabled = ?", *filter.Enabled)
	}
	if filter.Mode != "" {
		query = query.Where("mode = ?", filter.Mode)
	}
	if filter.PathPrefix != "" {
		prefix := normalizeVFSPathPrefix(filter.PathPrefix)
		query = query.Where(`mount_path = ? OR mount_path LIKE ? ESCAPE '\'`, prefix, pathChildrenLikePattern(prefix))
	}
	if !filter.IncludeHidden {
		query = query.Where("mount_path <> ''")
	}

	var models []VFSMountModel
	if err := query.Order("sort_order asc, id asc").Find(&models).Error; err != nil {
		return nil, normalizeGormError(err)
	}
	items := make([]*entity.VFSMount, 0, len(models))
	for i := range models {
		items = append(items, vfsMountEntityFromModel(&models[i]))
	}
	return items, nil
}

// UpsertByMountPath 基于 mount_path 创建或更新 VFS mount。
func (r *VFSMountRepository) UpsertByMountPath(ctx context.Context, mount *entity.VFSMount) error {
	model := vfsMountModelFromEntity(mount)
	if err := dbFor(ctx, r.db).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "mount_path"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"source_id", "node_id", "root_locator_json", "mode",
				"is_enabled", "sort_order", "updated_at",
			}),
		}).
		Create(model).Error; err != nil {
		return normalizeGormError(err)
	}

	saved, err := r.FindByMountPath(ctx, mount.MountPath)
	if err != nil {
		return err
	}
	*mount = *saved
	return nil
}

// VFSTagRepository 提供 VFS tag 与 node/tag 绑定仓储实现。
type VFSTagRepository struct {
	db *gormpkg.DB
}

// NewVFSTagRepository 创建 VFS tag 仓储。
func NewVFSTagRepository(db *gormpkg.DB) *VFSTagRepository {
	return &VFSTagRepository{db: db}
}

// Create 创建 VFS tag。
func (r *VFSTagRepository) Create(ctx context.Context, tag *entity.VFSTag) error {
	model := vfsTagModelFromEntity(tag)
	if err := dbFor(ctx, r.db).Create(model).Error; err != nil {
		return normalizeGormError(err)
	}
	*tag = *vfsTagEntityFromModel(model)
	return nil
}

// Update 更新 VFS tag。
func (r *VFSTagRepository) Update(ctx context.Context, tag *entity.VFSTag) error {
	model := vfsTagModelFromEntity(tag)
	result := dbFor(ctx, r.db).
		Model(&VFSTagModel{}).
		Where("id = ?", tag.ID).
		Select("*").
		Omit("ID", "OwnerUserID", "CreatedAt").
		Updates(model)
	if result.Error != nil {
		return normalizeGormError(result.Error)
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}
	return nil
}

// Delete 删除 VFS tag，并清理 node/tag 绑定。
func (r *VFSTagRepository) Delete(ctx context.Context, id uint) error {
	return dbFor(ctx, r.db).Transaction(func(tx *gormpkg.DB) error {
		result := tx.Delete(&VFSTagModel{}, id)
		if result.Error != nil {
			return normalizeGormError(result.Error)
		}
		if result.RowsAffected == 0 {
			return domainrepo.ErrNotFound
		}
		if err := tx.Where("tag_id = ?", id).Delete(&VFSNodeTagModel{}).Error; err != nil {
			return normalizeGormError(err)
		}
		return nil
	})
}

// FindByID 按 ID 查询 VFS tag。
func (r *VFSTagRepository) FindByID(ctx context.Context, id uint) (*entity.VFSTag, error) {
	var model VFSTagModel
	if err := dbFor(ctx, r.db).First(&model, id).Error; err != nil {
		return nil, normalizeGormNotFound(err)
	}
	return vfsTagEntityFromModel(&model), nil
}

// FindByOwnerAndName 按 owner/name 查询 VFS tag。
func (r *VFSTagRepository) FindByOwnerAndName(ctx context.Context, ownerUserID uint, name string) (*entity.VFSTag, error) {
	var model VFSTagModel
	if err := dbFor(ctx, r.db).
		Where("owner_user_id = ? AND name = ?", ownerUserID, name).
		First(&model).Error; err != nil {
		return nil, normalizeGormNotFound(err)
	}
	return vfsTagEntityFromModel(&model), nil
}

// List 按条件列出 VFS tags。
func (r *VFSTagRepository) List(ctx context.Context, filter domainrepo.VFSTagListFilter) ([]*entity.VFSTag, error) {
	query := dbFor(ctx, r.db).Model(&VFSTagModel{}).Where("owner_user_id = ?", filter.OwnerUserID)
	if filter.Name != "" {
		query = query.Where("name = ?", filter.Name)
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}

	var models []VFSTagModel
	if err := query.Order("name asc, id asc").Find(&models).Error; err != nil {
		return nil, normalizeGormError(err)
	}
	items := make([]*entity.VFSTag, 0, len(models))
	for i := range models {
		items = append(items, vfsTagEntityFromModel(&models[i]))
	}
	return items, nil
}

// UpsertByOwnerAndName 基于 owner_user_id/name 创建或更新 VFS tag。
func (r *VFSTagRepository) UpsertByOwnerAndName(ctx context.Context, tag *entity.VFSTag) error {
	model := vfsTagModelFromEntity(tag)
	if err := dbFor(ctx, r.db).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "owner_user_id"}, {Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{"color", "updated_at"}),
		}).
		Create(model).Error; err != nil {
		return normalizeGormError(err)
	}

	saved, err := r.FindByOwnerAndName(ctx, tag.OwnerUserID, tag.Name)
	if err != nil {
		return err
	}
	*tag = *saved
	return nil
}

// AttachToNode 绑定 tag 到 VFS node，重复绑定保持幂等。
func (r *VFSTagRepository) AttachToNode(ctx context.Context, binding *entity.VFSNodeTag) error {
	model := vfsNodeTagModelFromEntity(binding)
	if err := dbFor(ctx, r.db).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(model).Error; err != nil {
		return normalizeGormError(err)
	}
	*binding = *vfsNodeTagEntityFromModel(model)
	return nil
}

// DetachFromNode 解除 VFS node/tag 绑定。
func (r *VFSTagRepository) DetachFromNode(ctx context.Context, nodeID, tagID uint) error {
	result := dbFor(ctx, r.db).
		Where("node_id = ? AND tag_id = ?", nodeID, tagID).
		Delete(&VFSNodeTagModel{})
	if result.Error != nil {
		return normalizeGormError(result.Error)
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}
	return nil
}

// ListTagsForNode 列出指定 VFS node 绑定的 tags。
func (r *VFSTagRepository) ListTagsForNode(ctx context.Context, nodeID uint) ([]*entity.VFSTag, error) {
	bindings, err := r.ListBindingsForNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if len(bindings) == 0 {
		return []*entity.VFSTag{}, nil
	}

	tagIDs := make([]uint, 0, len(bindings))
	for _, binding := range bindings {
		tagIDs = append(tagIDs, binding.TagID)
	}

	var models []VFSTagModel
	if err := dbFor(ctx, r.db).
		Where("id IN ?", tagIDs).
		Order("name asc, id asc").
		Find(&models).Error; err != nil {
		return nil, normalizeGormError(err)
	}
	items := make([]*entity.VFSTag, 0, len(models))
	for i := range models {
		items = append(items, vfsTagEntityFromModel(&models[i]))
	}
	return items, nil
}

// ListBindingsForNode 列出指定 VFS node 的 tag 绑定元数据。
func (r *VFSTagRepository) ListBindingsForNode(ctx context.Context, nodeID uint) ([]*entity.VFSNodeTag, error) {
	var models []VFSNodeTagModel
	if err := dbFor(ctx, r.db).
		Where("node_id = ?", nodeID).
		Order("created_at asc, tag_id asc").
		Find(&models).Error; err != nil {
		return nil, normalizeGormError(err)
	}
	items := make([]*entity.VFSNodeTag, 0, len(models))
	for i := range models {
		items = append(items, vfsNodeTagEntityFromModel(&models[i]))
	}
	return items, nil
}

// ListNodeIDsByTag 列出绑定指定 tag 的 VFS node ID。
func (r *VFSTagRepository) ListNodeIDsByTag(ctx context.Context, tagID uint) ([]uint, error) {
	var models []VFSNodeTagModel
	if err := dbFor(ctx, r.db).
		Where("tag_id = ?", tagID).
		Order("node_id asc").
		Find(&models).Error; err != nil {
		return nil, normalizeGormError(err)
	}
	nodeIDs := make([]uint, 0, len(models))
	for _, model := range models {
		nodeIDs = append(nodeIDs, model.NodeID)
	}
	return nodeIDs, nil
}

func applyVFSNodeListFilter(query *gormpkg.DB, filter domainrepo.VFSNodeListFilter) *gormpkg.DB {
	if !filter.IncludeDeleted {
		query = query.Where("is_deleted = ?", false)
	}
	if filter.Kind != "" {
		query = query.Where("kind = ?", filter.Kind)
	}
	if filter.MountID != nil {
		query = query.Where("mount_id = ?", *filter.MountID)
	}
	if filter.SourceID != nil {
		query = query.Where("source_id = ?", *filter.SourceID)
	}
	if filter.SyncState != "" {
		query = query.Where("sync_state = ?", filter.SyncState)
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	return query
}

func normalizeVFSPathPrefix(pathPrefix string) string {
	prefix := strings.TrimRight(strings.TrimSpace(pathPrefix), "/")
	if prefix == "" {
		return "/"
	}
	return prefix
}

func pathChildrenLikePattern(path string) string {
	prefix := normalizeVFSPathPrefix(path)
	if prefix == "/" {
		return "/%"
	}
	return escapeLikePattern(prefix) + "/%"
}

func escapeLikePattern(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

func vfsNodeEntitiesFromModels(models []VFSNodeModel) []*entity.VFSNode {
	items := make([]*entity.VFSNode, 0, len(models))
	for i := range models {
		items = append(items, vfsNodeEntityFromModel(&models[i]))
	}
	return items
}

func vfsNodeModelFromEntity(node *entity.VFSNode) *VFSNodeModel {
	return &VFSNodeModel{
		ID:               node.ID,
		ParentID:         node.ParentID,
		Name:             node.Name,
		Path:             node.Path,
		Kind:             node.Kind,
		MountID:          node.MountID,
		ObjectID:         node.ObjectID,
		SourceID:         node.SourceID,
		ProviderItemID:   node.ProviderItemID,
		ProviderParentID: node.ProviderParentID,
		Size:             node.Size,
		MimeType:         node.MimeType,
		ETag:             node.ETag,
		Checksum:         node.Checksum,
		SyncState:        node.SyncState,
		IsDeleted:        node.IsDeleted,
		CreatedBy:        node.CreatedBy,
		UpdatedBy:        node.UpdatedBy,
		CreatedAt:        node.CreatedAt,
		UpdatedAt:        node.UpdatedAt,
		IndexedAt:        node.IndexedAt,
		LastSeenAt:       node.LastSeenAt,
	}
}

func vfsNodeEntityFromModel(model *VFSNodeModel) *entity.VFSNode {
	return &entity.VFSNode{
		ID:               model.ID,
		ParentID:         model.ParentID,
		Name:             model.Name,
		Path:             model.Path,
		Kind:             model.Kind,
		MountID:          model.MountID,
		ObjectID:         model.ObjectID,
		SourceID:         model.SourceID,
		ProviderItemID:   model.ProviderItemID,
		ProviderParentID: model.ProviderParentID,
		Size:             model.Size,
		MimeType:         model.MimeType,
		ETag:             model.ETag,
		Checksum:         model.Checksum,
		SyncState:        model.SyncState,
		IsDeleted:        model.IsDeleted,
		CreatedBy:        model.CreatedBy,
		UpdatedBy:        model.UpdatedBy,
		CreatedAt:        model.CreatedAt,
		UpdatedAt:        model.UpdatedAt,
		IndexedAt:        model.IndexedAt,
		LastSeenAt:       model.LastSeenAt,
	}
}

func storageObjectModelFromEntity(object *entity.StorageObject) *StorageObjectModel {
	return &StorageObjectModel{
		ID:          object.ID,
		SourceID:    object.SourceID,
		DriverType:  object.DriverType,
		LocatorType: object.LocatorType,
		LocatorJSON: jsonObject(object.LocatorJSON),
		Size:        object.Size,
		ETag:        object.ETag,
		Checksum:    object.Checksum,
		MimeType:    object.MimeType,
		Status:      object.Status,
		CreatedAt:   object.CreatedAt,
		UpdatedAt:   object.UpdatedAt,
	}
}

func storageObjectEntityFromModel(model *StorageObjectModel) *entity.StorageObject {
	return &entity.StorageObject{
		ID:          model.ID,
		SourceID:    model.SourceID,
		DriverType:  model.DriverType,
		LocatorType: model.LocatorType,
		LocatorJSON: model.LocatorJSON,
		Size:        model.Size,
		ETag:        model.ETag,
		Checksum:    model.Checksum,
		MimeType:    model.MimeType,
		Status:      model.Status,
		CreatedAt:   model.CreatedAt,
		UpdatedAt:   model.UpdatedAt,
	}
}

func vfsMountModelFromEntity(mount *entity.VFSMount) *VFSMountModel {
	return &VFSMountModel{
		ID:              mount.ID,
		SourceID:        mount.SourceID,
		NodeID:          mount.NodeID,
		MountPath:       mount.MountPath,
		RootLocatorJSON: jsonObject(mount.RootLocatorJSON),
		Mode:            mount.Mode,
		IsEnabled:       mount.IsEnabled,
		SortOrder:       mount.SortOrder,
		CreatedAt:       mount.CreatedAt,
		UpdatedAt:       mount.UpdatedAt,
	}
}

func vfsMountEntityFromModel(model *VFSMountModel) *entity.VFSMount {
	return &entity.VFSMount{
		ID:              model.ID,
		SourceID:        model.SourceID,
		NodeID:          model.NodeID,
		MountPath:       model.MountPath,
		RootLocatorJSON: model.RootLocatorJSON,
		Mode:            model.Mode,
		IsEnabled:       model.IsEnabled,
		SortOrder:       model.SortOrder,
		CreatedAt:       model.CreatedAt,
		UpdatedAt:       model.UpdatedAt,
	}
}

func vfsTagModelFromEntity(tag *entity.VFSTag) *VFSTagModel {
	return &VFSTagModel{
		ID:          tag.ID,
		OwnerUserID: tag.OwnerUserID,
		Name:        tag.Name,
		Color:       tag.Color,
		CreatedAt:   tag.CreatedAt,
		UpdatedAt:   tag.UpdatedAt,
	}
}

func vfsTagEntityFromModel(model *VFSTagModel) *entity.VFSTag {
	return &entity.VFSTag{
		ID:          model.ID,
		OwnerUserID: model.OwnerUserID,
		Name:        model.Name,
		Color:       model.Color,
		CreatedAt:   model.CreatedAt,
		UpdatedAt:   model.UpdatedAt,
	}
}

func vfsNodeTagModelFromEntity(binding *entity.VFSNodeTag) *VFSNodeTagModel {
	return &VFSNodeTagModel{
		NodeID:    binding.NodeID,
		TagID:     binding.TagID,
		CreatedBy: binding.CreatedBy,
		CreatedAt: binding.CreatedAt,
	}
}

func vfsNodeTagEntityFromModel(model *VFSNodeTagModel) *entity.VFSNodeTag {
	return &entity.VFSNodeTag{
		NodeID:    model.NodeID,
		TagID:     model.TagID,
		CreatedBy: model.CreatedBy,
		CreatedAt: model.CreatedAt,
	}
}
