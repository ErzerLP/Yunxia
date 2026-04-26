import { useCapabilityGuard } from '@/hooks/useCapability'

export function CapabilityRoute({ cap, children }: { cap: string; children: React.ReactNode }) {
  const { allowed, isLoading } = useCapabilityGuard(cap)
  if (isLoading) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }
  return allowed ? <>{children}</> : null
}
