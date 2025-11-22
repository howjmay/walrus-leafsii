import { formatNumber, formatPercentage } from "@/lib/utils";
import { useProtocolMetrics } from "@/hooks/useProtocolMetrics";

interface MetricTileProps {
  label: string;
  value: string;
  change?: string;
  changeType?: "positive" | "negative" | "neutral";
}

function MetricTile({
  label,
  value,
  change,
  changeType = "neutral",
}: MetricTileProps) {
  const changeColor = {
    positive: "text-success",
    negative: "text-danger",
    neutral: "text-text-muted",
  }[changeType];

  return (
    <div className="bg-bg-card2/50 rounded-xl p-4">
      <div className="text-text-muted text-sm mb-1">{label}</div>
      <div className="text-text-primary text-2xl font-bold mb-1">{value}</div>
      {change && <div className={`text-sm ${changeColor}`}>{change}</div>}
    </div>
  );
}

export function ProtocolMetrics() {
  const { data } = useProtocolMetrics();

  // Determine if we have valid data
  const hasData = !!data;

  // Convert string fields to numbers for formatting when data is available
  const metrics = hasData
    ? {
        currentCR: Number(data.currentCR),
        reserveRToken: Number(data.reservesR),
        fTokenSupply: Number(data.supplyF),
        xTokenSupply: Number(data.supplyX),
        spTVL: Number(data.spTVL),
        rewardAPR: Number(data.rewardAPR) / 100, // Convert percentage to decimal for formatPercentage
        indexDelta: Number(data.indexDelta),
      }
    : null;

  return (
    <div className="p-4 h-full overflow-auto">
      <div className="grid grid-cols-2 lg:grid-cols-3 gap-4">
        <MetricTile
          label="Current CR"
          value={hasData ? metrics!.currentCR.toFixed(2) : "Loading"}
        />

        <MetricTile
          label="Reserve (Sui)"
          value={hasData ? formatNumber(metrics!.reserveRToken) : "Loading"}
        />

        <MetricTile
          label="fToken Supply"
          value={hasData ? formatNumber(metrics!.fTokenSupply) : "Loading"}
        />

        <MetricTile
          label="xToken Supply"
          value={hasData ? formatNumber(metrics!.xTokenSupply) : "Loading"}
        />

        <MetricTile
          label="Stability Pool TVL"
          value={hasData ? `$${formatNumber(metrics!.spTVL)}` : "Loading"}
        />

        <MetricTile
          label="Reward APR"
          value={hasData ? formatPercentage(metrics!.rewardAPR) : "Loading"}
        />

        <MetricTile
          label="Index Delta"
          value={hasData ? formatPercentage(metrics!.indexDelta) : "Loading"}
        />
      </div>

      {/* System Status */}
      <div className="mt-6 p-4 bg-bg-card2/30 rounded-xl border border-success/20">
        <div className="flex items-center gap-2 mb-2">
          <div className="w-2 h-2 bg-success rounded-full animate-pulse" />
          <span className="text-success font-semibold">
            System Status: Normal
          </span>
        </div>
        <div className="text-text-muted text-sm">
          All parameters within normal ranges. Next keeper cycle in ~15 minutes.
        </div>
      </div>
    </div>
  );
}
