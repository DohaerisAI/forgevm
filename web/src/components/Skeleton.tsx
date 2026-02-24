export function StatCardSkeleton() {
  return (
    <div className="card animate-fade-in">
      <div className="flex items-start justify-between">
        <div className="space-y-2">
          <div className="skeleton h-4 w-24" />
          <div className="skeleton h-8 w-16" />
          <div className="skeleton h-3 w-20" />
        </div>
        <div className="skeleton w-12 h-12 rounded-xl" />
      </div>
    </div>
  );
}

export function SandboxCardSkeleton() {
  return (
    <div className="card !p-0 overflow-hidden animate-fade-in">
      <div className="flex items-center gap-4 px-5 py-4">
        <div className="skeleton w-10 h-10 rounded-lg" />
        <div className="flex-1 space-y-2">
          <div className="skeleton h-4 w-32" />
          <div className="skeleton h-3 w-48" />
        </div>
        <div className="skeleton h-6 w-16 rounded-full" />
        <div className="skeleton h-4 w-24" />
      </div>
    </div>
  );
}

export function ProviderCardSkeleton() {
  return (
    <div className="card !p-0 overflow-hidden animate-fade-in">
      <div className="flex items-center gap-4 px-5 py-4">
        <div className="skeleton w-12 h-12 rounded-xl" />
        <div className="flex-1 space-y-2">
          <div className="skeleton h-5 w-28" />
          <div className="skeleton h-3 w-64" />
        </div>
        <div className="skeleton h-9 w-24 rounded-lg" />
      </div>
    </div>
  );
}

export function TemplateCardSkeleton() {
  return (
    <div className="card animate-fade-in space-y-3">
      <div className="flex items-start justify-between">
        <div className="space-y-2">
          <div className="skeleton h-5 w-32" />
          <div className="skeleton h-3 w-56" />
        </div>
        <div className="skeleton w-8 h-8 rounded-lg" />
      </div>
      <div className="flex gap-2">
        <div className="skeleton h-5 w-20 rounded-full" />
        <div className="skeleton h-5 w-16 rounded-full" />
      </div>
    </div>
  );
}
