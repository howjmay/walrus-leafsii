export const featureFlags = {
  xtoken: true, // xToken minting/redeeming - enabled
  limit: true,   // Limit orders - enabled for demo
  twap: true,    // TWAP orders - enabled for demo
  telemetry: false, // Analytics tracking
} as const

export type FeatureFlag = keyof typeof featureFlags