export type QueryValue = string | number | boolean | null | undefined
export type Query = Record<string, QueryValue>

export type RequestMethod = 'GET' | 'POST' | 'PUT' | 'DELETE'

export class ApiError extends Error {
  readonly status: number
  readonly code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

export interface RequestOptions {
  method?: RequestMethod
  query?: Query
  body?: unknown
  authRedirect?: boolean
}

let unauthorizedHandler: (() => void) | null = null

export function setUnauthorizedHandler(handler: (() => void) | null): void {
  unauthorizedHandler = handler
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function buildUrl(path: string, query: Query | undefined): string {
  const url = new URL(path, window.location.origin)
  if (query) {
    for (const [key, value] of Object.entries(query)) {
      if (value === undefined || value === null || value === '') continue
      url.searchParams.set(key, String(value))
    }
  }
  return url.toString()
}

export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  let response: Response
  try {
    response = await fetch(buildUrl(path, options.query), {
      method: options.method ?? 'GET',
      headers: options.body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
      credentials: 'include',
    })
  } catch {
    throw new ApiError(0, 'network_error', '网络连接失败，请稍后重试')
  }

  let payload: unknown
  try {
    const text = await response.text()
    payload = text ? (JSON.parse(text) as unknown) : undefined
  } catch {
    throw new ApiError(response.status, 'bad_response', '响应数据格式错误')
  }

  if (!response.ok) {
    const err = isRecord(payload) ? payload['error'] : undefined
    const errRecord = isRecord(err) ? err : undefined
    const code = typeof errRecord?.['code'] === 'string' ? errRecord['code'] : 'unknown_error'
    const message =
      typeof errRecord?.['message'] === 'string'
        ? errRecord['message']
        : `请求失败（HTTP ${response.status}）`
    if (response.status === 401 && options.authRedirect !== false) {
      unauthorizedHandler?.()
    }
    throw new ApiError(response.status, code, message)
  }

  return payload as T
}

export function errorMessage(error: unknown): string {
  if (error instanceof ApiError) return error.message
  if (error instanceof Error) return error.message
  return '未知错误'
}
