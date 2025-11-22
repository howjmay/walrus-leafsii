import { useState, useEffect, useRef, useMemo } from 'react'
import { ChevronDown, BarChart3, Wifi, WifiOff } from 'lucide-react'
import { createChart, IChartApi, ISeriesApi, CandlestickData } from 'lightweight-charts'
import { usePriceFeed } from '../../hooks/usePriceFeed'
import { calculate24hChange } from '../../utils/candles'
import type { ChartInterval } from '../../types/price'

type Interval = ChartInterval
export type Pair = 'SUI/USD' | 'ETH/USD'

const intervals = [
  { value: '15m' as const, label: '15m' },
  { value: '1h' as const, label: '1h' },
  { value: '4h' as const, label: '4h' },
  { value: '1d' as const, label: '1d' },
] as const

const pairs = [
  { value: 'SUI/USD', label: 'SUI/USD' },
  { value: 'ETH/USD', label: 'ETH/USD' },
] as const

interface PriceChartProps {
  selectedPair: Pair
  onPairChange: (pair: Pair) => void
}

export function PriceChart({ selectedPair, onPairChange }: PriceChartProps) {
  const [selectedInterval, setSelectedInterval] = useState<Interval>('15m')
  const [isPairDropdownOpen, setIsPairDropdownOpen] = useState(false)
  
  const chartRef = useRef<HTMLDivElement>(null)
  const chartApiRef = useRef<IChartApi | null>(null)
  const candlestickSeriesRef = useRef<ISeriesApi<'Candlestick'> | null>(null)
  const chartInitializedRef = useRef(false)
  const lastTimeRef = useRef<number | null>(null)

  // Subtle health badge can be hidden via env
  const showHealthBadge = (import.meta.env.VITE_SHOW_HEALTH_BADGE ?? 'true') !== 'false'

  // Get the selected pair data
  const selectedPairData = useMemo(() => 
    pairs.find(p => p.value === selectedPair) || pairs[0], 
    [selectedPair]
  )

  // Use the price feed hook for real-time data
  const { candles, currentPrice, isConnected, isMocked, error } = usePriceFeed({
    pair: selectedPair,
    interval: selectedInterval,
    enabled: true,
  })

  // Initialize chart
  useEffect(() => {
    if (!chartRef.current) return

    // Create chart instance
    const chart = createChart(chartRef.current, {
      width: chartRef.current.clientWidth,
      height: chartRef.current.clientHeight,
      layout: {
        background: { color: '#1F2937' },
        textColor: '#9CA3AF',
      },
      grid: {
        vertLines: { color: '#273244' },
        horzLines: { color: '#273244' },
      },
      crosshair: {
        mode: 1,
      },
      rightPriceScale: {
        borderColor: '#4B5563',
      },
      timeScale: {
        borderColor: '#4B5563',
        timeVisible: true,
        secondsVisible: false,
      },
    })

    // Create candlestick series
    const candlestickSeries = chart.addCandlestickSeries({
      upColor: '#22C55E',
      downColor: '#EF4444',
      borderDownColor: '#EF4444',
      borderUpColor: '#22C55E',
      wickDownColor: '#EF4444',
      wickUpColor: '#22C55E',
    })

    chartApiRef.current = chart
    candlestickSeriesRef.current = candlestickSeries

    // Handle resize
    const handleResize = () => {
      if (chartRef.current && chart) {
        chart.applyOptions({
          width: chartRef.current.clientWidth,
          height: chartRef.current.clientHeight,
        })
      }
    }
    
    const resizeObserver = new ResizeObserver(handleResize)
    resizeObserver.observe(chartRef.current)

    return () => {
      resizeObserver.disconnect()
      chart.remove()
      chartApiRef.current = null
      candlestickSeriesRef.current = null
    }
  }, [])

  // Reset chart state when switching pair or interval so history re-renders correctly
  useEffect(() => {
    chartInitializedRef.current = false
    lastTimeRef.current = null
    candlestickSeriesRef.current?.setData([])
  }, [selectedPair, selectedInterval])

  // Initialize data once, then only update/append the last candle to avoid flicker
  useEffect(() => {
    const series = candlestickSeriesRef.current
    if (!series || candles.length === 0) return

    // First load: set the full dataset once
    if (!chartInitializedRef.current) {
      const chartData: CandlestickData[] = candles.map(c => ({
        time: c.time as any,
        open: c.open,
        high: c.high,
        low: c.low,
        close: c.close,
      }))
      series.setData(chartData)
      lastTimeRef.current = candles[candles.length - 1].time
      chartInitializedRef.current = true
      return
    }

    // Subsequent updates: only update or append the latest candle
    const last = candles[candles.length - 1]
    if (!last) return
    const prevTime = lastTimeRef.current
    const lastPoint: CandlestickData = {
      time: last.time as any,
      open: last.open,
      high: last.high,
      low: last.low,
      close: last.close,
    }

    if (prevTime == null || last.time > prevTime) {
      // New candle (next bucket)
      series.update(lastPoint)
      lastTimeRef.current = last.time
    } else if (last.time === prevTime) {
      // Same candle bucket: update only
      series.update(lastPoint)
    } else {
      // Out-of-order; reinitialize defensively
      const chartData: CandlestickData[] = candles.map(c => ({
        time: c.time as any,
        open: c.open,
        high: c.high,
        low: c.low,
        close: c.close,
      }))
      series.setData(chartData)
      lastTimeRef.current = candles[candles.length - 1].time
    }
  }, [candles])

  // Calculate 24h price change
  const priceChange = useMemo(() => 
    calculate24hChange(candles, currentPrice), 
    [currentPrice, candles]
  )

  return (
    <div className="h-full flex flex-col">
      {/* Controls */}
      <div className="flex items-center justify-between p-4 border-b border-border-subtle">
        {/* Pair Selector */}
        <div className="relative">
          <button
            onClick={() => setIsPairDropdownOpen(!isPairDropdownOpen)}
            className="flex items-center gap-2 px-3 py-2 bg-bg-input rounded-lg hover:bg-bg-card2 transition-colors"
          >
            <BarChart3 className="w-4 h-4" />
            <span className="text-text-primary font-medium">
              {selectedPairData.label}
            </span>
            <ChevronDown className={`w-4 h-4 transition-transform ${isPairDropdownOpen ? 'rotate-180' : ''}`} />
          </button>

          {isPairDropdownOpen && (
            <div className="absolute top-full mt-1 left-0 min-w-full bg-bg-card border border-border-subtle rounded-lg shadow-card z-10">
              {pairs.map((pair) => (
                <button
                  key={pair.value}
                  onClick={() => {
                    onPairChange(pair.value)
                    setIsPairDropdownOpen(false)
                  }}
                  className={`w-full px-3 py-2 text-left hover:bg-bg-card2 transition-colors first:rounded-t-lg last:rounded-b-lg ${
                    pair.value === selectedPair ? 'bg-bg-card2 text-brand-primary' : 'text-text-primary'
                  }`}
                >
                  {pair.label}
                </button>
              ))}
            </div>
          )}
        </div>

        {/* Interval Selector */}
        <div className="flex bg-bg-input rounded-lg p-1">
          {intervals.map((interval) => (
            <button
              key={interval.value}
              onClick={() => setSelectedInterval(interval.value)}
              className={`px-3 py-1 text-sm font-medium rounded-md transition-colors ${
                selectedInterval === interval.value
                  ? 'bg-brand-primary text-text-onBrand'
                  : 'text-text-secondary hover:text-text-primary'
              }`}
            >
              {interval.label}
            </button>
          ))}
        </div>
      </div>

      {/* Chart */}
      <div className="flex-1 relative">
        <div ref={chartRef} className="absolute inset-4" />
        
        {/* Price Info Overlay */}
        <div className="absolute top-4 left-4 bg-bg-card/80 backdrop-blur-sm border border-border-subtle rounded-lg p-3">
          <div className="flex items-center gap-2 mb-1">
            <span className="text-text-muted text-xs">Current Price</span>
            <div className="flex items-center gap-1">
              {isConnected ? (
                <Wifi className="w-3 h-3 text-success" />
              ) : (
                <WifiOff className="w-3 h-3 text-error" />
              )}
              {isMocked && (
                <span className="text-xs px-1.5 py-0.5 bg-yellow-500/20 text-yellow-500 rounded">
                  DEMO
                </span>
              )}
            </div>
          </div>
          <div className="text-text-primary text-lg font-bold">
            ${currentPrice?.toFixed(4) || '-.----'}
          </div>
          <div className={`text-sm ${priceChange.percentage >= 0 ? 'text-success' : 'text-error'}`}>
            {priceChange.percentage >= 0 ? '+' : ''}
            {priceChange.percentage.toFixed(2)}% (24h)
          </div>
          {error && (
            <div className="text-text-muted text-xs mt-1">{error}</div>
          )}
        </div>

        {/* Subtle health badge (toggle with VITE_SHOW_HEALTH_BADGE=false) */}
        {showHealthBadge && (
          <div className="absolute bottom-4 left-4 text-[10px] px-2 py-0.5 rounded-full border border-border-subtle/60 bg-bg-card/60 text-text-secondary/80 backdrop-blur-sm select-none">
            {isConnected && !isMocked && (
              <span className="text-success/80">LIVE</span>
            )}
            {isConnected && isMocked && (
              <span className="text-yellow-500/80">MOCK</span>
            )}
            {!isConnected && (
              <span className="text-text-muted/70">OFFLINE</span>
            )}
          </div>
        )}
      </div>

      {/* Overlay to close dropdown */}
      {isPairDropdownOpen && (
        <div
          className="fixed inset-0 z-5"
          onClick={() => setIsPairDropdownOpen(false)}
        />
      )}
    </div>
  )
}
