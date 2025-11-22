export interface Tick {
  symbol: string;
  price: number;
  ts: number; // milliseconds since epoch
}

export interface Candle {
  time: number; // unix timestamp in seconds
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
}

export interface CandleResponse {
  data: Candle[];
  mocked?: boolean;
}

export type ChartInterval = '1m' | '5m' | '15m' | '1h' | '4h' | '1d';

export interface PriceFeedState {
  candles: Candle[];
  currentPrice: number | null;
  isConnected: boolean;
  isMocked: boolean;
  error: string | null;
  lastUpdate: number | null;
}