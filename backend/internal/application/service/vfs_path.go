package service

import (
	"path"
	"strings"
)

// normalizeMountPath 规范化挂载路径。
func normalizeMountPath(input string) (string, error) {
	return normalizeVirtualPath(input)
}

// splitParentName 将路径拆分为父路径与当前名称。
func splitParentName(input string) (string, string, error) {
	normalizedPath, err := normalizeVirtualPath(input)
	if err != nil {
		return "", "", err
	}
	if normalizedPath == "/" {
		return "/", "", nil
	}

	return path.Dir(normalizedPath), path.Base(normalizedPath), nil
}

// isSubPath 判断 target 是否位于 base 之下，包含自身。
func isSubPath(base string, target string) bool {
	normalizedBase, err := normalizeVirtualPath(base)
	if err != nil {
		return false
	}
	normalizedTarget, err := normalizeVirtualPath(target)
	if err != nil {
		return false
	}
	if normalizedBase == "/" {
		return strings.HasPrefix(normalizedTarget, "/")
	}

	return normalizedTarget == normalizedBase || strings.HasPrefix(normalizedTarget, normalizedBase+"/")
}

// resolveVirtualPathByLongestPrefix 基于最长前缀解析虚拟路径。
func resolveVirtualPathByLongestPrefix(virtualPath string, mounts []MountEntry) (ResolvedPath, error) {
	normalizedPath, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return ResolvedPath{}, err
	}

	resolved := ResolvedPath{
		VirtualPath: normalizedPath,
	}

	bestIndex := -1
	bestMountPath := ""
	for index, mount := range mounts {
		normalizedMountPath, normalizeErr := normalizeMountPath(mount.MountPath)
		if normalizeErr != nil {
			return ResolvedPath{}, normalizeErr
		}
		if !isSubPath(normalizedMountPath, normalizedPath) {
			continue
		}
		if len(normalizedMountPath) > len(bestMountPath) {
			bestIndex = index
			bestMountPath = normalizedMountPath
		}
	}

	if bestIndex >= 0 {
		resolved.MatchedMountPath = bestMountPath
		resolved.Source = mounts[bestIndex].Source
		resolved.IsRealMount = true
		resolved.InnerPath = strings.TrimPrefix(normalizedPath, bestMountPath)
		if resolved.InnerPath == "" {
			resolved.InnerPath = "/"
		} else if !strings.HasPrefix(resolved.InnerPath, "/") {
			resolved.InnerPath = "/" + resolved.InnerPath
		}
		return resolved, nil
	}

	for _, mount := range mounts {
		normalizedMountPath, normalizeErr := normalizeMountPath(mount.MountPath)
		if normalizeErr != nil {
			return ResolvedPath{}, normalizeErr
		}
		if normalizedMountPath != normalizedPath && isSubPath(normalizedPath, normalizedMountPath) {
			resolved.IsPureVirtual = true
			return resolved, nil
		}
	}

	return resolved, nil
}
