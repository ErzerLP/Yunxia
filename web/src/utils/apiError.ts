import { ApiRequestError } from '@/api/client'

const ERROR_CODE_MESSAGES: Record<string, string> = {
  SOURCE_OPERATION_UNSUPPORTED: '当前存储源暂不支持该操作；PikPak 暂不支持暂停/恢复和永久删除，请改用取消任务或移入回收站。',
  FILE_ALREADY_EXISTS: '目标位置已存在同名文件，请改名后重试。',
  NAME_CONFLICT: '目标位置已存在同名条目，或名称与挂载点冲突。',
  CLOUD_AUTH_FAILED: '网盘账号认证失败，请检查账号、密码或 refresh token。',
  CLOUD_TOKEN_INVALID: '网盘登录态已失效，请更新 refresh token 或重新填写账号密码。',
  CLOUD_CAPTCHA_REQUIRED: 'PikPak 需要完成安全验证，请管理员完成验证后回填 captcha_token。',
  CLOUD_RATE_LIMITED: '网盘服务请求过于频繁，请稍后再试。',
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
}

export function getApiErrorCode(error: unknown) {
  if (error instanceof ApiRequestError) return error.code
  return ''
}

export function getRawErrorMessage(error: unknown) {
  return error instanceof Error ? error.message : ''
}

export function getApiErrorMessage(error: unknown, fallback = '操作失败') {
  const code = getApiErrorCode(error)
  if (code && ERROR_CODE_MESSAGES[code]) {
    return ERROR_CODE_MESSAGES[code]
  }

  const rawMessage = getRawErrorMessage(error)
  if (!rawMessage) return fallback

  const upperMessage = rawMessage.toUpperCase()
  const matchedCode = Object.keys(ERROR_CODE_MESSAGES).find((candidate) => upperMessage.includes(candidate))
  if (matchedCode) {
    return ERROR_CODE_MESSAGES[matchedCode]
  }

  return rawMessage
}
