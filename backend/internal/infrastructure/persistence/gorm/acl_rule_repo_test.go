package gorm

import (
	"context"
	"testing"
	"time"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

func TestACLRuleRepositoryPersistsVFSNodeID(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewACLRuleRepository(db)
	nodeID := uint(42)
	rule := &entity.ACLRule{
		SourceID:          7,
		VFSNodeID:         &nodeID,
		Path:              "/docs",
		VirtualPath:       "/mount/docs",
		SubjectType:       "user",
		SubjectID:         11,
		Effect:            "allow",
		Priority:          100,
		Read:              true,
		InheritToChildren: true,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	if err := repo.Create(ctx, rule); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if rule.VFSNodeID == nil || *rule.VFSNodeID != nodeID {
		t.Fatalf("Create() did not round-trip vfs_node_id=%d: %#v", nodeID, rule)
	}

	found, err := repo.FindByID(ctx, rule.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if found.VFSNodeID == nil || *found.VFSNodeID != nodeID {
		t.Fatalf("FindByID() vfs_node_id = %#v, want %d", found.VFSNodeID, nodeID)
	}

	listed, err := repo.List(ctx, domainrepo.ACLRuleFilter{SourceID: 7, VFSNodeID: &nodeID})
	if err != nil {
		t.Fatalf("List(vfs_node_id) error = %v", err)
	}
	if len(listed) != 1 || listed[0].ID != rule.ID {
		t.Fatalf("List(vfs_node_id) = %#v, want created rule", listed)
	}
}
