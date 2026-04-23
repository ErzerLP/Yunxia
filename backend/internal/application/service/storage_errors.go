package service

import "errors"

var (
	// ErrSourceDriverUnsupported 表示当前驱动暂不支持。
	ErrSourceDriverUnsupported = errors.New("source driver unsupported")
	// ErrConfigInvalid 表示存储源配置不合法。
	ErrConfigInvalid = errors.New("config invalid")
	// ErrSourceNameConflict 表示存储源名称冲突。
	ErrSourceNameConflict = errors.New("source name conflict")
	// ErrSourceConnectionFailed 表示存储源连接测试失败。
	ErrSourceConnectionFailed = errors.New("source connection failed")
	// ErrSourceReadOnly 表示存储源只读。
	ErrSourceReadOnly = errors.New("source read only")
	// ErrSourceInUse 表示存储源正在被使用。
	ErrSourceInUse = errors.New("source in use")
	// ErrPathInvalid 表示路径非法。
	ErrPathInvalid = errors.New("path invalid")
	// ErrFileNotFound 表示文件不存在。
	ErrFileNotFound = errors.New("file not found")
	// ErrFileAlreadyExists 表示文件已存在。
	ErrFileAlreadyExists = errors.New("file already exists")
	// ErrFileNameInvalid 表示文件名非法。
	ErrFileNameInvalid = errors.New("file name invalid")
	// ErrFileIsDirectory 表示目标是目录。
	ErrFileIsDirectory = errors.New("file is directory")
	// ErrFileMoveConflict 表示移动冲突。
	ErrFileMoveConflict = errors.New("file move conflict")
	// ErrFileCopyConflict 表示复制冲突。
	ErrFileCopyConflict = errors.New("file copy conflict")
	// ErrUploadSessionNotFound 表示上传会话不存在。
	ErrUploadSessionNotFound = errors.New("upload session not found")
	// ErrUploadChunkConflict 表示上传分片冲突。
	ErrUploadChunkConflict = errors.New("upload chunk conflict")
	// ErrUploadFinishIncomplete 表示上传未完成。
	ErrUploadFinishIncomplete = errors.New("upload finish incomplete")
	// ErrUploadHashMismatch 表示上传完成后的哈希不匹配。
	ErrUploadHashMismatch = errors.New("upload hash mismatch")
	// ErrUploadInvalidState 表示上传状态不允许当前操作。
	ErrUploadInvalidState = errors.New("upload invalid state")
	// ErrUploadTooLarge 表示上传超限。
	ErrUploadTooLarge = errors.New("upload too large")
	// ErrTaskInvalidState 表示任务状态不允许当前操作。
	ErrTaskInvalidState = errors.New("task invalid state")
	// ErrUserNameConflict 表示用户名冲突。
	ErrUserNameConflict = errors.New("user name conflict")
	// ErrUserRoleInvalid 表示用户角色非法。
	ErrUserRoleInvalid = errors.New("user role invalid")
	// ErrUserStatusInvalid 表示用户状态非法。
	ErrUserStatusInvalid = errors.New("user status invalid")
	// ErrRoleAssignmentForbidden 表示角色分配越权。
	ErrRoleAssignmentForbidden = errors.New("role assignment forbidden")
	// ErrLastSuperAdminForbidden 表示禁止移除最后一个激活的 super admin。
	ErrLastSuperAdminForbidden = errors.New("last super admin forbidden")
	// ErrACLSubjectTypeInvalid 表示 ACL 规则主体类型非法。
	ErrACLSubjectTypeInvalid = errors.New("acl subject type invalid")
	// ErrACLEffectInvalid 表示 ACL 规则 effect 非法。
	ErrACLEffectInvalid = errors.New("acl effect invalid")
	// ErrACLPermissionsInvalid 表示 ACL 权限集合非法。
	ErrACLPermissionsInvalid = errors.New("acl permissions invalid")
	// ErrACLDenied 表示 ACL 运行时拒绝当前访问。
	ErrACLDenied = errors.New("acl denied")
	// ErrPermissionDenied 表示当前身份无权执行该动作。
	ErrPermissionDenied = errors.New("permission denied")
	// ErrShareExpired 表示分享链接已过期。
	ErrShareExpired = errors.New("share expired")
	// ErrSharePasswordRequired 表示访问分享时缺少密码。
	ErrSharePasswordRequired = errors.New("share password required")
	// ErrSharePasswordInvalid 表示访问分享时密码错误。
	ErrSharePasswordInvalid = errors.New("share password invalid")
)
