import { cn } from "@/lib/cn";

type QuotaBarProps = {
  label: string;
  value?: number;
  threshold?: number;
};

export function QuotaBar({ label, value = 0, threshold = 90 }: QuotaBarProps): JSX.Element {
  const safeValue = Math.max(0, Math.min(100, Math.round(value)));
  const color =
    safeValue >= threshold ? "bg-red-400" : safeValue >= 80 ? "bg-orange-400" : safeValue >= 60 ? "bg-amber-400" : "bg-emerald-400";

  return (
    <div>
      <div className="mb-2 flex items-center justify-between text-sm">
        <span className="text-muted-foreground">{label}</span>
        <span className="font-medium">{safeValue}%</span>
      </div>
      <div className="h-2.5 overflow-hidden rounded-full bg-secondary">
        <div className={cn("h-full rounded-full transition-all", color)} style={{ width: `${safeValue}%` }} />
      </div>
    </div>
  );
}
