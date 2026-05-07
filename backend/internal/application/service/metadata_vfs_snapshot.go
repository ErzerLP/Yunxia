package service

import (
	"context"
	"strings"
)

func resolveMetadataVFSNodeID(ctx context.Context, reader metadataVFSReader, virtualPath string) uint {
	if reader == nil || strings.TrimSpace(virtualPath) == "" {
		return 0
	}
	normalizedPath, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return 0
	}
	node, err := reader.ResolveNode(ctx, normalizedPath)
	if err != nil || node == nil {
		return 0
	}
	return node.ID
}
