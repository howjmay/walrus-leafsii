import type { Tick, Candle, ChartInterval } from '../types/price';

// Convert interval string to milliseconds
export function intervalToMs(interval: ChartInterval): number {
  switch (interval) {
    case '1m': return 60 * 1000;
    case '5m': return 5 * 60 * 1000;
    case '15m': return 15 * 60 * 1000;
    case '1h': return 60 * 60 * 1000;
    case '4h': return 4 * 60 * 60 * 1000;
    case '1d': return 24 * 60 * 60 * 1000;
    default: return 60 * 60 * 1000; // Default to 1h
  }
}

// Align timestamp to interval boundary
export function alignTime(timestamp: number, intervalMs: number): number {
  return Math.floor(timestamp / intervalMs) * intervalMs;
}

// Generate mock continuation candles when live data is unavailable
export function generateMockCandles(
  lastCandle: Candle, 
  interval: ChartInterval, 
  count: number = 10
): Candle[] {
  const intervalMs = intervalToMs(interval);
  const intervalSec = intervalMs / 1000;
  const candles: Candle[] = [];
  
  let lastPrice = lastCandle.close;
  let currentTime = lastCandle.time; // seconds aligned to bucket start
  
  for (let i = 1; i <= count; i++) {
    currentTime += intervalSec; // next bucket start (seconds)
    
    // Generate small random price movement
    const volatility = 0.002; // 0.2%
    const change = (Math.random() - 0.5) * volatility;
    const open = lastPrice;
    const close = open * (1 + change);
    
    // Generate high/low with some spread
    const spread = Math.abs(change) + 0.001; // Minimum spread
    const high = Math.max(open, close) * (1 + spread);
    const low = Math.min(open, close) * (1 - spread);
    
    const candle: Candle = {
      time: currentTime,
      open,
      high,
      low,
      close,
      volume: lastCandle.volume * (0.8 + Math.random() * 0.4), // ±20% volume variation
    };
    
    candles.push(candle);
    lastPrice = close;
  }
  
  return candles;
}

// Candle aggregator that maintains state per symbol+timeframe
export class CandleAggregator {
  private currentCandles = new Map<string, Candle>();

  private getKey(symbol: string, interval: ChartInterval, periodStart: number): string {
    const intervalMs = intervalToMs(interval);
    return `${symbol}|${intervalMs}|${periodStart}`;
  }

  // Process a tick and return the result indicating if a new candle was created
  processTick(symbol: string, tick: Tick, interval: ChartInterval): {
    candle: Candle;
    isNewCandle: boolean;
    previousCandle?: Candle;
  } {
    const intervalMs = intervalToMs(interval);
    const tickTime = tick.ts;
    const periodStart = alignTime(tickTime, intervalMs);
    const bucketTime = Math.floor(periodStart / 1000); // Convert to seconds
    
    const key = this.getKey(symbol, interval, periodStart);
    const existingCandle = this.currentCandles.get(key);
    
    if (!existingCandle) {
      // Check if we need to finalize a previous candle
      const prevPeriodStart = periodStart - intervalMs;
      const prevKey = this.getKey(symbol, interval, prevPeriodStart);
      const previousCandle = this.currentCandles.get(prevKey);
      
      // Clean up previous candle
      if (previousCandle) {
        this.currentCandles.delete(prevKey);
      }
      
      // Create new candle
      const newCandle: Candle = {
        time: bucketTime,
        open: tick.price,
        high: tick.price,
        low: tick.price,
        close: tick.price,
        volume: 0,
      };
      
      this.currentCandles.set(key, newCandle);
      
      return {
        candle: newCandle,
        isNewCandle: true,
        previousCandle,
      };
    }
    
    // Update existing candle in-place
    existingCandle.high = Math.max(existingCandle.high, tick.price);
    existingCandle.low = Math.min(existingCandle.low, tick.price);
    existingCandle.close = tick.price;
    
    return {
      candle: existingCandle,
      isNewCandle: false,
    };
  }

  // Handle late/out-of-order ticks
  processLateTick(tick: Tick, interval: ChartInterval, existingCandles: Candle[]): {
    updatedCandle?: Candle;
    candleIndex?: number;
  } {
    const intervalMs = intervalToMs(interval);
    const periodStart = alignTime(tick.ts, intervalMs);
    const bucketTime = Math.floor(periodStart / 1000);
    
    // Find the matching candle in historical data
    const candleIndex = existingCandles.findIndex(c => c.time === bucketTime);
    if (candleIndex >= 0) {
      const candle = existingCandles[candleIndex];
      const updatedCandle: Candle = {
        ...candle,
        high: Math.max(candle.high, tick.price),
        low: Math.min(candle.low, tick.price),
        close: tick.price, // Assumes this is the "latest" price for this bucket
      };
      return { updatedCandle, candleIndex };
    }
    
    return {};
  }

  // Clear state for symbol+interval (useful when switching timeframes)
  clearForSymbolInterval(symbol: string, interval: ChartInterval): void {
    const intervalMs = intervalToMs(interval);
    const prefix = `${symbol}|${intervalMs}|`;
    
    for (const key of this.currentCandles.keys()) {
      if (key.startsWith(prefix)) {
        this.currentCandles.delete(key);
      }
    }
  }

  // Get current candle for debugging
  getCurrentCandle(symbol: string, interval: ChartInterval, periodStart: number): Candle | undefined {
    const key = this.getKey(symbol, interval, periodStart);
    return this.currentCandles.get(key);
  }
}


// Compute the start and end (boundary) of the current interval bucket given now
export function currentBucketBounds(nowMs: number, interval: ChartInterval): { startSec: number; endSec: number } {
  const ms = intervalToMs(interval)
  const startMs = alignTime(nowMs, ms)
  return {
    startSec: Math.floor(startMs / 1000),
    endSec: Math.floor((startMs + ms) / 1000),
  }
}


// Calculate 24h price change
export function calculate24hChange(candles: Candle[], currentPrice?: number | null): { delta: number; percentage: number } {
  if (!currentPrice || candles.length < 2) return { delta: 0, percentage: 0 };
  
  // Try to find candle from ~24h ago (1440 minutes = 1 day)
  const hoursAgo24 = Math.min(candles.length - 1, 24); // Approximate
  const referenceCandle = candles[candles.length - 1 - hoursAgo24] || candles[0];
  
  const delta = currentPrice - referenceCandle.close;
  const percentage = (delta / referenceCandle.close) * 100;
  
  return { delta, percentage };
}
