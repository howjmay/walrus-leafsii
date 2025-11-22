let runtimeApiBase: string | null = null

export function setRuntimeApiBase(base: string) {
  runtimeApiBase = base
}

export function getApiBase(): string {
  const clean = (v?: string | null) =>
    (v && v.trim().replace(/\/$/, '')) || ''

  if (runtimeApiBase) return clean(runtimeApiBase)

  const rewriteLocalhost = (base: string) => {
    if (typeof window === 'undefined') return base
    try {
      const url = new URL(base)
      const host = url.hostname
      const winHost = window.location.hostname
      const isLocalHost = (h: string) => ['localhost', '127.0.0.1', '[::1]'].includes(h)
      if (!isLocalHost(winHost) && isLocalHost(host)) {
        url.hostname = winHost
        return clean(url.toString())
      }
    } catch {
      return base
    }
    return base
  }

  const envBase = clean(import.meta.env.VITE_API_BASE)
  if (envBase) return rewriteLocalhost(envBase)

  // In dev, prefer the local backend even when the dev server is exposed over LAN.
  if (import.meta.env.DEV) {
    return rewriteLocalhost('http://localhost:8080')
  }

  // If deployed somewhere other than localhost, default to same origin so
  // hosted frontends talk to their co-located backend.
  if (typeof window !== 'undefined') {
    const url = new URL(window.location.href)
    const host = url.hostname
    const isLocal =
      host === 'localhost' || host === '127.0.0.1' || host === '[::1]'
    if (!isLocal) {
      return clean(window.location.origin)
    }
  }

  // Local dev fallback
  return 'http://localhost:8080'
}

export function apiUrl(path: string): string {
  const base = getApiBase()
  const p = path.startsWith('/') ? path : `/${path}`
  return `${base}${p}`
}

export function getApiBaseForNetwork(network?: string | null): string {
  const clean = (v?: string | null) =>
    (v && v.trim().replace(/\/$/, '')) || ''

  const rewriteLocalhost = (base: string) => {
    if (typeof window === 'undefined') return base
    try {
      const url = new URL(base)
      const host = url.hostname
      const winHost = window.location.hostname
      const isLocalHost = (h: string) => ['localhost', '127.0.0.1', '[::1]'].includes(h)
      if (!isLocalHost(winHost) && isLocalHost(host)) {
        url.hostname = winHost
        return clean(url.toString())
      }
    } catch {
      return base
    }
    return base
  }

  const networkBase =
    network === 'mainnet'
      ? clean(import.meta.env.VITE_MAINNET_API_BASE)
      : network === 'testnet'
        ? clean(import.meta.env.VITE_TESTNET_API_BASE)
        : network === 'localnet'
          ? clean(import.meta.env.VITE_LOCALNET_API_BASE)
          : ''

  if (networkBase) return rewriteLocalhost(networkBase)
  return getApiBase()
}

export function apiUrlForNetwork(path: string, network?: string | null): string {
  const base = getApiBaseForNetwork(network)
  const p = path.startsWith('/') ? path : `/${path}`
  return `${base}${p}`
}
