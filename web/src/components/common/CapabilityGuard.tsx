import { useHasCapability, useHasAnyCapability } from '@/hooks/useCapability'

interface CapabilityGuardProps {
  cap: string
  fallback?: React.ReactNode
  children: React.ReactNode
}

export function CapabilityGuard({ cap, fallback = null, children }: CapabilityGuardProps) {
  const hasCap = useHasCapability(cap)
  return hasCap ? <>{children}</> : <>{fallback}</>
}

interface CapabilityAnyGuardProps {
  caps: string[]
  fallback?: React.ReactNode
  children: React.ReactNode
}

export function CapabilityAnyGuard({ caps, fallback = null, children }: CapabilityAnyGuardProps) {
  const hasAny = useHasAnyCapability(caps)
  return hasAny ? <>{children}</> : <>{fallback}</>
}
