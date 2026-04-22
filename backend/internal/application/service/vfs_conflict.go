package service

import "context"

func ensureWritableNameAvailable(
	ctx context.Context,
	targetPath string,
	mounts []MountEntry,
	fileDrivers map[string]FileDriver,
) error {
	conflict, err := hasMountNameConflict(targetPath, mounts)
	if err != nil {
		return err
	}
	if conflict {
		return ErrNameConflict
	}

	resolved, err := resolveVirtualPathByLongestPrefix(targetPath, mounts)
	if err != nil {
		return err
	}
	if !resolved.IsRealMount || resolved.Source == nil {
		return nil
	}

	exists, err := sourcePathExists(ctx, resolved.Source, resolved.InnerPath, fileDrivers)
	if err != nil {
		return err
	}
	if exists {
		return ErrNameConflict
	}

	return nil
}

func hasMountNameConflict(targetPath string, mounts []MountEntry) (bool, error) {
	normalizedTargetPath, err := normalizeVirtualPath(targetPath)
	if err != nil {
		return false, err
	}
	if normalizedTargetPath == "/" {
		return false, nil
	}

	for _, mount := range mounts {
		normalizedMountPath, normalizeErr := normalizeMountPath(mount.MountPath)
		if normalizeErr != nil {
			return false, normalizeErr
		}
		if normalizedMountPath == normalizedTargetPath {
			return true, nil
		}
		if isSubPath(normalizedTargetPath, normalizedMountPath) {
			return true, nil
		}
	}

	return false, nil
}
