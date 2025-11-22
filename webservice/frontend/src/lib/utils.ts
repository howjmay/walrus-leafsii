import { type ClassValue, clsx } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function formatNumber(value: number, decimals: number = 2): string {
  if (value >= 1e9) {
    return (value / 1e9).toFixed(1) + 'B'
  }
  if (value >= 1e6) {
    return (value / 1e6).toFixed(1) + 'M'
  }
  if (value >= 1e3) {
    return (value / 1e3).toFixed(1) + 'K'
  }
  return value.toFixed(decimals)
}

export function formatPercentage(value: number, decimals: number = 2): string {
  return `${(value * 100).toFixed(decimals)}%`
}

export function formatCurrency(value: number, currency: string = 'USD', decimals: number = 2): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency,
    minimumFractionDigits: decimals,
    maximumFractionDigits: decimals,
  }).format(value)
}

export function truncateAddress(address: string, start: number = 6, end: number = 4): string {
  if (address.length <= start + end) return address
  return `${address.slice(0, start)}...${address.slice(-end)}`
}

export function formatTokenAmount(
  raw: string | bigint, 
  decimals = 18, 
  opts?: { minFraction?: number; maxFraction?: number }
): string {
  const { minFraction = 2, maxFraction = 6 } = opts || {}
  
  // Convert to BigInt
  const rawBigInt = typeof raw === 'string' ? BigInt(raw) : raw
  
  // Calculate the divisor (10^decimals)
  const divisor = BigInt(10 ** decimals)
  
  // Get integer and fractional parts
  const integerPart = rawBigInt / divisor
  const fractionalPart = rawBigInt % divisor
  
  // Convert integer part to string with thousands separators
  const integerStr = integerPart.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',')
  
  // Handle fractional part
  if (fractionalPart === 0n) {
    // No fractional part, but respect minFraction
    if (minFraction > 0) {
      return integerStr + '.' + '0'.repeat(minFraction)
    }
    return integerStr
  }
  
  // Convert fractional part to string and pad with zeros
  const fractionalStr = fractionalPart.toString().padStart(decimals, '0')
  
  // Trim trailing zeros, but keep at least minFraction digits
  let trimmedFractional = fractionalStr.replace(/0+$/, '')
  if (trimmedFractional.length < minFraction) {
    trimmedFractional = trimmedFractional.padEnd(minFraction, '0')
  }
  
  // Limit to maxFraction digits
  if (trimmedFractional.length > maxFraction) {
    trimmedFractional = trimmedFractional.slice(0, maxFraction)
  }
  
  // Return formatted result
  return trimmedFractional.length > 0 ? integerStr + '.' + trimmedFractional : integerStr
}