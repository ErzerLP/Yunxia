export function getFileIconClass(mimeType: string, isDir: boolean): string {
  if (isDir) return 'folder'
  if (mimeType.startsWith('image/')) return 'image'
  if (mimeType.startsWith('video/')) return 'video'
  if (mimeType.startsWith('audio/')) return 'audio'
  if (mimeType.includes('pdf')) return 'pdf'
  if (mimeType.includes('zip') || mimeType.includes('rar') || mimeType.includes('7z')) return 'archive'
  if (mimeType.includes('doc') || mimeType.includes('word')) return 'document'
  if (mimeType.includes('xls') || mimeType.includes('excel') || mimeType.includes('sheet')) return 'spreadsheet'
  if (mimeType.includes('ppt') || mimeType.includes('presentation')) return 'presentation'
  if (mimeType.startsWith('text/') || mimeType.includes('json') || mimeType.includes('javascript')) return 'code'
  return 'file'
}

export function getFileExtension(name: string): string {
  const parts = name.split('.')
  if (parts.length <= 1) return ''
  return parts[parts.length - 1].toLowerCase()
}

export function isPreviewable(mimeType: string): boolean {
  if (mimeType.startsWith('image/')) return true
  if (mimeType.startsWith('video/')) return true
  if (mimeType.startsWith('audio/')) return true
  if (mimeType.startsWith('text/')) return true
  if (mimeType.includes('pdf')) return true
  if (mimeType.includes('json')) return true
  if (mimeType.includes('javascript')) return true
  return false
}
