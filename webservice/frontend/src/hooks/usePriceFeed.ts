import { useState, useEffect, useRef, useCallback } from 'react';
import type { Tick, Candle, ChartInterval, PriceFeedState, CandleResponse } from '../types/price';
import { CandleAggregator, generateMockCandles, intervalToMs } from '../utils/candles';
import { apiUrl } from '../utils/api';

interface UsePriceFeedOptions {
  pair: string;
  interval: ChartInterval;
  enabled?: boolean;
}

export function usePriceFeed({ pair, interval, enabled = true }: UsePriceFeedOptions) {
  const [state, setState] = useState<PriceFeedState>({
    candles: [],
    currentPrice: null,
    isConnected: false,
    isMocked: false,
    error: null,
    lastUpdate: null,
  });

  const eventSourceRef = useRef<EventSource | null>(null);
  const candleAggregatorRef = useRef<CandleAggregator>(new CandleAggregator());
  const mockTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const initialLoadRef = useRef(false);
  const lastLiveUpdateRef = useRef<number>(Date.now());

  // Get symbol for SSE subscription (map UI pair to provider symbol)
  const getSymbolForSSE = (uiPair: string): string => {
    switch (uiPair.toUpperCase()) {
      case 'SUI/USD':
      case 'SUI/USDT':
      case 'SUI/FTOKEN':
        return 'SUIUSDT';
      case 'ETH/USD':
      case 'ETH/USDT':
        return 'ETHUSDT';
      default:
        return 'SUIUSDT'; // Default fallback
    }
  };

  // Fetch initial candle history
  const fetchInitialCandles = useCallback(async () => {
    if (!enabled || initialLoadRef.current) return;

    try {
      const response = await fetch(apiUrl(`v1/candles?pair=${encodeURIComponent(pair)}&interval=${interval}&limit=500`));
      if (!response.ok) {
        throw new Error(`Failed to fetch candles: ${response.statusText}`);
      }
      
      const result: CandleResponse = await response.json();
      const mocked = result.mocked || response.headers.get('X-Mocked') === 'true';
      
      setState(prev => ({
        ...prev,
        candles: result.data,
        isMocked: mocked,
        error: null,
        currentPrice: result.data.length > 0 ? result.data[result.data.length - 1].close : null,
      }));
      
      // Initialize aggregator with latest candle data if available
      if (result.data.length > 0) {
        // No need to seed aggregator - it will create candles as ticks arrive
      }
      
      initialLoadRef.current = true;
    } catch (error) {
      console.error('Failed to fetch initial candles:', error);
      // Build a minimal mocked history so the chart always shows something
      const now = Date.now()
      const ms = intervalToMs(interval)
      const startSec = Math.floor((now - ms * 200) / 1000)
      const seed: Candle = { time: startSec, open: 1.0, high: 1.0, low: 0.999, close: 1.0, volume: 1000 }
      const mocked = generateMockCandles(seed, interval, 200)
      setState(prev => ({
        ...prev,
        candles: mocked,
        currentPrice: mocked[mocked.length - 1]?.close ?? 1.0,
        error: error instanceof Error ? error.message : 'Failed to fetch initial data',
        isMocked: true,
      }))
    }
  }, [pair, interval, enabled]);

  // Start mock tick simulation when live data is unavailable
  // Simulates individual ticks to test the aggregation logic properly
  const startMockContinuation = useCallback(() => {
    if (mockTimeoutRef.current) { clearTimeout(mockTimeoutRef.current) }

    let mockTickCount = 0;
    const symbol = getSymbolForSSE(pair);

    const scheduleNextTick = () => {
      setState(prev => {
        if (prev.candles.length === 0) return prev
        
        // Get current price from last candle
        const lastCandle = prev.candles[prev.candles.length - 1];
        const basePrice = prev.currentPrice || lastCandle.close;
        
        // Generate small price movement (simulate realistic tick)
        const volatility = 0.001; // 0.1% volatility
        const priceChange = (Math.random() - 0.5) * volatility;
        const newPrice = basePrice * (1 + priceChange);
        
        // Create mock tick with current timestamp
        const mockTick = {
          symbol,
          price: newPrice,
          ts: Date.now(), // Current time to test real-time aggregation
        };
        
        // Process through our aggregator
        const result = candleAggregatorRef.current.processTick(symbol, mockTick, interval);
        
        // Console logging for debugging
        const tickTime = new Date(mockTick.ts).toISOString();
        const bucketTime = new Date(result.candle.time * 1000).toISOString();
        console.log(`[MOCK TICK ${mockTickCount}] ${tickTime} (${mockTick.price.toFixed(4)}) → ${result.isNewCandle ? 'NEW' : 'UPDATE'} bucket ${bucketTime}`);
        
        // Update state based on aggregation result
        const updatedCandles = [...prev.candles];
        
        if (result.isNewCandle) {
          // Add new candle
          updatedCandles.push(result.candle);
          if (updatedCandles.length > 500) {
            updatedCandles.shift();
          }
        } else {
          // Update existing candle in-place
          const lastIndex = updatedCandles.length - 1;
          if (lastIndex >= 0 && updatedCandles[lastIndex].time === result.candle.time) {
            updatedCandles[lastIndex] = result.candle;
          }
        }
        
        return {
          ...prev,
          candles: updatedCandles,
          currentPrice: mockTick.price,
          isMocked: true,
          lastUpdate: Date.now(),
        };
      });

      mockTickCount++;
      
      // Schedule next tick every 2-5 seconds (simulate realistic tick frequency)
      const nextTickDelay = 2000 + Math.random() * 3000;
      mockTimeoutRef.current = setTimeout(scheduleNextTick, nextTickDelay);
    };

    scheduleNextTick();
  }, [interval, pair])

  // Setup SSE connection for real-time updates
  const setupSSEConnection = useCallback(() => {
    if (!enabled) return;

    const symbol = getSymbolForSSE(pair);
    const eventSource = new EventSource(apiUrl(`v1/stream?topics=price&symbol=${encodeURIComponent(symbol)}`));
    eventSourceRef.current = eventSource;

    eventSource.onopen = () => {
      setState(prev => ({
        ...prev,
        isConnected: true,
        error: null,
      }));
      lastLiveUpdateRef.current = Date.now();
    };

    eventSource.addEventListener('price_update', (event) => {
      try {
        const tick: Tick = JSON.parse(event.data);
        const symbol = getSymbolForSSE(pair);
        
        // Process tick through aggregator
        const result = candleAggregatorRef.current.processTick(symbol, tick, interval);
        
        // Console logging for real SSE ticks
        const tickTime = new Date(tick.ts).toISOString();
        const bucketTime = new Date(result.candle.time * 1000).toISOString();
        console.log(`[LIVE TICK] ${tickTime} (${tick.price}) → ${result.isNewCandle ? 'NEW' : 'UPDATE'} bucket ${bucketTime}`);
        
        setState(prev => {
          const updatedCandles = [...prev.candles];
          
          if (result.isNewCandle) {
            // Add new candle
            updatedCandles.push(result.candle);
            // Maintain buffer size
            if (updatedCandles.length > 500) {
              updatedCandles.shift();
            }
          } else {
            // Update existing candle in-place
            const lastIndex = updatedCandles.length - 1;
            if (lastIndex >= 0 && updatedCandles[lastIndex].time === result.candle.time) {
              updatedCandles[lastIndex] = result.candle;
            } else {
              // Fallback: handle out-of-order by finding the right candle
              const candleIndex = updatedCandles.findIndex(c => c.time === result.candle.time);
              if (candleIndex >= 0) {
                updatedCandles[candleIndex] = result.candle;
              } else {
                // This shouldn't happen, but add as new candle if not found
                updatedCandles.push(result.candle);
              }
            }
          }
          
          return {
            ...prev,
            candles: updatedCandles,
            currentPrice: tick.price,
            isMocked: false,
            lastUpdate: Date.now(),
          };
        });

        lastLiveUpdateRef.current = Date.now();
        
        // Clear mock continuation if we're receiving live data
        if (mockTimeoutRef.current) {
          clearTimeout(mockTimeoutRef.current);
          mockTimeoutRef.current = null;
        }
        
      } catch (error) {
        console.error('Failed to parse SSE message:', error);
      }
    });

    eventSource.onerror = (error) => {
      console.error('SSE connection error:', error);
      setState(prev => ({
        ...prev,
        isConnected: false,
        error: 'Connection lost. Using mock data...',
      }));
      
      // Start mock continuation after connection failure
      startMockContinuation();
    };

    // Check for SSE timeout and start mock if no updates
    const timeoutCheck = setTimeout(() => {
      const timeSinceLastUpdate = Date.now() - lastLiveUpdateRef.current;
      if (timeSinceLastUpdate > 10000) { // 10 seconds timeout
        console.warn('No SSE updates received, starting mock continuation');
        startMockContinuation();
      }
    }, 10000);

    return () => {
      clearTimeout(timeoutCheck);
    };
  }, [enabled, pair, interval, startMockContinuation]);

  // Reset and refetch when pair or interval changes
  useEffect(() => {
    // Clear previous state
    setState({
      candles: [],
      currentPrice: null,
      isConnected: false,
      isMocked: false,
      error: null,
      lastUpdate: null,
    });
    
    // Clear aggregator state for this symbol+interval
    const symbol = getSymbolForSSE(pair);
    candleAggregatorRef.current.clearForSymbolInterval(symbol, interval);
    initialLoadRef.current = false;
    
    // Clear existing connections and timeouts
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
      eventSourceRef.current = null;
    }
    if (mockTimeoutRef.current) { clearTimeout(mockTimeoutRef.current); mockTimeoutRef.current = null }
    // no interval ticker to clear

    // Fetch initial data and setup SSE
    fetchInitialCandles().then(() => {
      const cleanupSSE = setupSSEConnection();
      return cleanupSSE;
    });
  }, [pair, interval, fetchInitialCandles, setupSSEConnection]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
      }
      if (mockTimeoutRef.current) { clearTimeout(mockTimeoutRef.current) }
      // no interval ticker to clear
    };
  }, []);

  return {
    candles: state.candles,
    currentPrice: state.currentPrice,
    isConnected: state.isConnected,
    isMocked: state.isMocked,
    error: state.error,
    lastUpdate: state.lastUpdate,
  };
}
