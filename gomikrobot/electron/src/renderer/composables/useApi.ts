import { useModeStore } from '../stores/mode'
import { useRemoteStore } from '../stores/remote'

/**
 * Composable for API calls that switches base URL depending on mode.
 * - Modes 1/2 (full/standalone): http://127.0.0.1:18791
 * - Mode 3 (remote): remote connection URL with bearer token
 */
export function useApi() {
  function getBaseUrl(): string {
    const modeStore = useModeStore()
    if (modeStore.currentMode === 'remote') {
      const remoteStore = useRemoteStore()
      return remoteStore.activeConnection?.url || 'http://127.0.0.1:18791'
    }
    return 'http://127.0.0.1:18791'
  }

  function getHeaders(): Record<string, string> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      Accept: 'application/json',
    }
    const modeStore = useModeStore()
    if (modeStore.currentMode === 'remote') {
      const remoteStore = useRemoteStore()
      const token = remoteStore.activeConnection?.token
      if (token) {
        headers['Authorization'] = `Bearer ${token}`
      }
    }
    return headers
  }

  async function get<T = any>(path: string): Promise<T> {
    const res = await fetch(`${getBaseUrl()}${path}`, {
      method: 'GET',
      headers: getHeaders(),
    })
    if (!res.ok) {
      throw new Error(`HTTP ${res.status}: ${await res.text()}`)
    }
    return res.json()
  }

  async function post<T = any>(path: string, body?: any): Promise<T> {
    const res = await fetch(`${getBaseUrl()}${path}`, {
      method: 'POST',
      headers: getHeaders(),
      body: body ? JSON.stringify(body) : undefined,
    })
    if (!res.ok) {
      throw new Error(`HTTP ${res.status}: ${await res.text()}`)
    }
    return res.json()
  }

  return { get, post, getBaseUrl }
}
