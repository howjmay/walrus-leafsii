import { TrendingUp, Clock, AlertTriangle } from "lucide-react";
import { Card } from "@/components/ui/Card";
import { useProtocolState } from "@/hooks/useProtocolState";

export function PxTicker() {
  const { data: protocolState, isLoading, error } = useProtocolState();

  const formatTimeAgo = (timestamp: number) => {
    const seconds = Math.floor(Date.now() / 1000 - timestamp);
    if (seconds < 60) return `${seconds}s ago`;
    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return `${minutes}m ago`;
    const hours = Math.floor(minutes / 60);
    return `${hours}h ago`;
  };

  if (isLoading) {
    return (
      <Card className="p-3">
        <div className="flex items-center gap-2">
          <TrendingUp className="w-4 h-4 text-brand-primary" />
          <span className="text-text-muted text-sm">Loading Px...</span>
        </div>
      </Card>
    );
  }

  if (error) {
    return (
      <Card className="p-3">
        <div className="flex items-center gap-2">
          <AlertTriangle className="w-4 h-4 text-danger" />
          <span className="text-danger text-sm">Failed to load Px</span>
        </div>
      </Card>
    );
  }

  const px = protocolState?.px || 0;
  const formattedPx = (px / 1000000).toFixed(4); // Convert from micro units to decimal

  return (
    <Card className="p-3">
      <div className="flex items-center">
        {/* Left: Label + icon */}
        <div className="flex items-center gap-2">
          <TrendingUp
            className="w-4 h-4 text-brand-primary"
            aria-hidden="true"
          />
          <span className="text-text-muted text-sm">Px Price</span>
        </div>

        {/* Right: Number + USD (right-aligned) */}
        <span className="ml-auto inline-flex items-baseline gap-2 text-text-primary font-semibold">
          <span className="tabular-nums">{formattedPx}</span>
          <span className="text-text-muted text-sm">USD</span>
        </span>
      </div>

      {/* Timestamp row (optional) */}
      {protocolState?.asOf && (
        <div className="mt-2 flex items-center justify-end gap-1">
          <Clock className="w-3 h-3 text-text-muted" aria-hidden="true" />
          <span className="text-text-muted text-xs">
            {formatTimeAgo(protocolState.asOf)}
          </span>
        </div>
      )}
    </Card>
  );
}
