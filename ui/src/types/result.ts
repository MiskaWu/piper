// Result<T> 錯誤處理基礎設施
// 參考 GM 專案的 Result 模式，適配 Piper 的 fetch-based API

export interface AppError {
  code: string
  message: string
  details?: unknown
}

export interface Success<T> {
  ok: true
  data: T
}

export interface Failure<E = AppError> {
  ok: false
  error: E
}

export type Result<T, E = AppError> = Success<T> | Failure<E>

export function success<T>(data: T): Success<T> {
  return { ok: true, data }
}

export function failure<E = AppError>(error: E): Failure<E> {
  return { ok: false, error }
}

/**
 * 將 fetch 錯誤轉換為 AppError。
 *
 * Piper 的 api.ts request() 會在 !res.ok 時拋出 Error(`${status}: ${text}`)，
 * 因此這裡解析該格式；其他錯誤（如 AbortError、TypeError）也一併處理。
 */
export function fromFetchError(error: unknown): AppError {
  if (error instanceof DOMException && error.name === 'AbortError') {
    return {
      code: 'TIMEOUT',
      message: '請求逾時',
      details: error,
    }
  }

  if (error instanceof TypeError) {
    // fetch 在網路不可用時拋出 TypeError
    return {
      code: 'NETWORK_ERROR',
      message: error.message || '網路錯誤',
      details: error,
    }
  }

  if (error instanceof Error) {
    // 嘗試解析 api.ts 的 `${status}: ${text}` 格式
    const match = error.message.match(/^(\d{3}):\s*(.*)$/)
    if (match) {
      return {
        code: match[1],
        message: match[2] || `HTTP ${match[1]}`,
        details: error,
      }
    }
    return {
      code: 'UNKNOWN',
      message: error.message,
      details: error,
    }
  }

  return {
    code: 'UNKNOWN',
    message: String(error),
    details: error,
  }
}

/**
 * 包裝 async 呼叫，統一回傳 Result<T>。
 * Services 層使用此函式取代 try/catch。
 */
export async function safeCall<T>(fn: () => Promise<T>): Promise<Result<T>> {
  try {
    const data = await fn()
    return success(data)
  } catch (error) {
    return failure(fromFetchError(error))
  }
}
