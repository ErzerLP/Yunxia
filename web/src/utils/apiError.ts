import { ApiRequestError } from '@/api/client'

const ERROR_CODE_MESSAGES: Record<string, string> = {
  VALIDATION_ERROR: '提交内容未通过校验，请检查表单字段后重试；如果问题仍存在，请刷新页面后重新选择目标。',
  SOURCE_OPERATION_UNSUPPORTED: '当前存储源暂不支持该操作；PikPak 暂不支持暂停/恢复和永久删除，请改用取消任务或移入回收站。',
  SOURCE_NOT_FOUND: '存储源或远端资源不存在，请检查目标是否已删除、挂载配置是否正确。',
  FILE_ALREADY_EXISTS: '目标位置已存在同名文件，请改名后重试。',
  NAME_CONFLICT: '目标位置已存在同名条目，或名称与挂载点冲突。',
  CLOUD_AUTH_FAILED: '网盘账号认证失败，请检查账号、密码或 refresh token。',
  CLOUD_TOKEN_INVALID: '网盘登录态已失效，请更新 refresh token 或重新填写账号密码。',
  CLOUD_CAPTCHA_REQUIRED: 'PikPak 需要完成安全验证，请管理员完成验证后回填 captcha_token。',
  CLOUD_RATE_LIMITED: '网盘服务请求过于频繁，请稍后再试。',
  CLOUD_REGION_BLOCKED: '当前网络区域暂不支持访问该网盘服务，请更换可用网络或代理后重试。',
  CLOUD_PROVIDER_UNAVAILABLE: '网盘服务暂时不可用，请稍后重试。',
  SOURCE_READ_ONLY: '目标存储源当前只读，不能执行写操作。',
  NO_BACKING_STORAGE: '目标虚拟目录没有唯一的实际存储源，请进入具体挂载目录后再操作。',
  PERMISSION_DENIED: '当前账号没有执行该操作的权限。',
  CAPABILITY_DENIED: '当前账号缺少执行该操作的能力权限。',
  PATH_INVALID: '路径无效，请检查目标目录或源配置。',
  CONFIG_INVALID: '配置不合法，请检查必填项和字段格式。',
  MOUNT_PATH_CONFLICT: '挂载路径已被其他存储源占用，请换一个挂载路径。',
  SOURCE_NAME_CONFLICT: '存储源名称已存在，请换一个名称。',
  SOURCE_DRIVER_UNSUPPORTED: '当前后端不支持该存储源驱动。',
  SOURCE_CONNECTION_FAILED: '存储源连接测试失败，请检查网络和配置。',
  METADATA_VFS_COMMIT_FAILED: '文件已写入底层存储，但目录索引提交失败，请稍后重试或联系管理员。',
  METADATA_VFS_MUTATION_SYNC_FAILED: '文件已写入底层存储，但同步目录索引失败，请刷新目录或稍后重试。',
  VFS_SYNC_CONFLICT: '目录刷新发现文件状态冲突，请稍后重试或联系管理员处理。',
  VFS_NODE_NOT_FOUND: '所选文件或目录节点已删除或不可解析，请重新选择路径。',
  FILE_NOT_FOUND: '目标不存在或当前不可用。',
  ACL_DENIED: '当前账号无权访问该路径。',
  TAG_INVALID: '标签名称或颜色不合法，请检查后重试。',
  TAG_NOT_FOUND: '标签不存在或已被删除。',
  TAG_BINDING_NOT_FOUND: '该文件未绑定此标签，可能已被其他操作移除。',
}

const DATABASE_ERROR_MESSAGE = '服务端数据库处理失败，请稍后重试或联系管理员；详细错误已记录在后端日志中。'
const PIKPAK_RESOURCE_NOT_FOUND_MESSAGE =
  'PikPak 连接失败：远端资源不存在或 Root Folder ID 不正确，请检查 PikPak 目录 ID、账号权限和代理配置；如果当前应触发人工验证但后端未返回 verification_url，请检查后端响应。'
const NO_BACKING_STORAGE_MESSAGE = ERROR_CODE_MESSAGES.NO_BACKING_STORAGE

function isDatabaseErrorMessage(message: string) {
  return (
    message.includes('sqlstate') ||
    message.includes(' sql ') ||
    message.includes('sql:') ||
    message.includes('pq:') ||
    message.includes('gorm') ||
    message.includes('database') ||
    message.includes('value too long for type') ||
    message.includes('duplicate key value') ||
    message.includes('unique constraint') ||
    message.includes('constraint failed') ||
    (message.includes('violates') && message.includes('constraint')) ||
    (message.includes('error:') && message.includes('sqlstate'))
  )
}

function isValidationErrorMessage(message: string) {
  return (
    message.includes('validation') && (
      message.includes('failed on the') ||
      message.includes('required') ||
      message.includes('binding') ||
      message.includes("key: '") ||
      message.includes('key:')
    )
  )
}

export function getApiErrorCode(error: unknown) {
  if (error instanceof ApiRequestError) return error.code
  return ''
}

export function getApiErrorDetails(error: unknown) {
  if (error instanceof ApiRequestError) return error.details ?? {}
  return {}
}

export function getApiErrorDetailString(error: unknown, key: string) {
  const value = getApiErrorDetails(error)[key]
  return typeof value === 'string' && value.trim() ? value.trim() : ''
}

export function getRawErrorMessage(error: unknown) {
  if (typeof error === 'string') return error
  return error instanceof Error ? error.message : ''
}

export function getApiErrorMessage(error: unknown, fallback = '操作失败') {
  const rawMessage = getRawErrorMessage(error)
  const normalizedMessage = rawMessage.toLowerCase()
  if (normalizedMessage.includes('no backing storage')) {
    return NO_BACKING_STORAGE_MESSAGE
  }
  if (normalizedMessage.includes('cloud captcha required') || normalizedMessage.includes('captcha required')) {
    return ERROR_CODE_MESSAGES.CLOUD_CAPTCHA_REQUIRED
  }
  if (normalizedMessage.includes('cloud captcha expired') || normalizedMessage.includes('captcha expired')) {
    return ERROR_CODE_MESSAGES.CLOUD_CAPTCHA_REQUIRED
  }
  if (normalizedMessage.includes('cloud auth failed') || normalizedMessage.includes('authentication failed')) {
    return ERROR_CODE_MESSAGES.CLOUD_AUTH_FAILED
  }
  if (normalizedMessage.includes('cloud token invalid') || normalizedMessage.includes('token invalid')) {
    return ERROR_CODE_MESSAGES.CLOUD_TOKEN_INVALID
  }
  if (normalizedMessage.includes('cloud rate limited') || normalizedMessage.includes('rate limited')) {
    return ERROR_CODE_MESSAGES.CLOUD_RATE_LIMITED
  }
  if (normalizedMessage.includes('cloud provider unavailable') || normalizedMessage.includes('provider unavailable')) {
    return ERROR_CODE_MESSAGES.CLOUD_PROVIDER_UNAVAILABLE
  }
  if (normalizedMessage.includes('cloud region blocked') || normalizedMessage.includes('region blocked')) {
    return ERROR_CODE_MESSAGES.CLOUD_REGION_BLOCKED
  }
  if (normalizedMessage.includes('metadata vfs commit failed')) {
    return ERROR_CODE_MESSAGES.METADATA_VFS_COMMIT_FAILED
  }
  if (normalizedMessage.includes('metadata vfs mutation sync failed')) {
    return ERROR_CODE_MESSAGES.METADATA_VFS_MUTATION_SYNC_FAILED
  }
  if (
    normalizedMessage.includes('source connection failed') &&
    normalizedMessage.includes('resource not found')
  ) {
    return PIKPAK_RESOURCE_NOT_FOUND_MESSAGE
  }

  const code = getApiErrorCode(error)
  if (code && ERROR_CODE_MESSAGES[code]) {
    return ERROR_CODE_MESSAGES[code]
  }

  if (isDatabaseErrorMessage(normalizedMessage)) {
    return DATABASE_ERROR_MESSAGE
  }
  if (isValidationErrorMessage(normalizedMessage)) {
    return ERROR_CODE_MESSAGES.VALIDATION_ERROR
  }

  if (!rawMessage) return fallback

  const upperMessage = rawMessage.toUpperCase()
  const matchedCode = Object.keys(ERROR_CODE_MESSAGES).find((candidate) => upperMessage.includes(candidate))
  if (matchedCode) {
    return ERROR_CODE_MESSAGES[matchedCode]
  }

  return rawMessage
}
