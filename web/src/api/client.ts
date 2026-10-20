// Обёртка над fetch по договору docs/API.md:
// - тела — JSON, ответ 204 — undefined;
// - каждый запрос, кроме GET/HEAD, несёт X-Requested-With: garage (ТБ-3);
// - ошибка API — тело {error, field?, retry_after?} → ApiError;
// - «нет связи» (fetch упал, истёк срок ожидания, 502–504 от nginx) → NetworkError.
import { isOnline, reportConnection } from './connection';

/** Сколько ждать ответа: без VPN соединение может висеть минуту и дольше. */
export const REQUEST_TIMEOUT_MS = 15_000;

/** Ошибка, которую вернул сервер: текст можно показывать как есть. */
export class ApiError extends Error {
  readonly status: number;
  readonly field: string | undefined;
  /** Через сколько секунд можно повторить (429). */
  readonly retryAfter: number | undefined;
  /** Сколько есть на складе (409 «не хватает» у расхода). */
  readonly available: number | undefined;

  constructor(status: number, message: string, field?: string, retryAfter?: number, available?: number) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.field = field;
    this.retryAfter = retryAfter;
    this.available = available;
  }
}

/**
 * Как пропала связь:
 * - offline — запрос не дошёл (fetch упал сразу, nginx не достучался до garage: 502/503);
 * - timeout — ответ не пришёл (истёк срок ожидания, 504): запрос мог уже выполниться.
 */
export type NetworkErrorKind = 'offline' | 'timeout';

/** Сервер недоступен: VPN выключен, сеть пропала или nginx не достучался до garage. */
export class NetworkError extends Error {
  readonly kind: NetworkErrorKind;

  constructor(kind: NetworkErrorKind = 'offline', message = 'Сервер недоступен') {
    super(message);
    this.name = 'NetworkError';
    this.kind = kind;
  }

  /** Изменение могло сохраниться на сервере, хотя ответа нет. */
  get maybeSaved(): boolean {
    return this.kind === 'timeout';
  }
}

export function isApiError(err: unknown, status?: number): err is ApiError {
  return err instanceof ApiError && (status === undefined || err.status === status);
}

export function isNetworkError(err: unknown): err is NetworkError {
  return err instanceof NetworkError;
}

type Method = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';

export type RequestOptions = {
  method?: Method;
  body?: unknown;
  /** Готовое JSON-тело (файл импорта): уходит как есть, без повторной сериализации. */
  rawBody?: string;
  /** Двоичное тело (фото позиции): уходит как есть, Content-Type — тип самого Blob. */
  binary?: Blob;
  signal?: AbortSignal;
  /** Сколько ждать ответа, мс; по умолчанию REQUEST_TIMEOUT_MS (большая загрузка ждёт дольше). */
  timeoutMs?: number;
  /**
   * Изменение, которое можно отправлять без связи (расход и шаблон: очередь, ФТ-12).
   * Остальные изменения при известном отсутствии связи не уходят в сеть вовсе.
   */
  offlineOk?: boolean;
};

const GATEWAY_STATUSES = new Set([502, 503, 504]);

export async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const method = opts.method ?? 'GET';
  const headers: Record<string, string> = { Accept: 'application/json' };
  if (method !== 'GET') headers['X-Requested-With'] = 'garage';
  let body: string | Blob | undefined;
  if (opts.binary !== undefined) {
    headers['Content-Type'] = opts.binary.type || 'application/octet-stream';
    body = opts.binary;
  } else if (opts.rawBody !== undefined) {
    headers['Content-Type'] = 'application/json';
    body = opts.rawBody;
  } else if (opts.body !== undefined) {
    headers['Content-Type'] = 'application/json';
    body = JSON.stringify(opts.body);
  }

  // Связи нет — изменение (кроме расхода и входа) не ждёт тайм-аута: «Нужна связь» сразу (ФТ-12.2.3).
  if (method !== 'GET' && !opts.offlineOk && !isOnline() && !path.startsWith('/api/auth/')) throw new NetworkError('offline');

  const timeout = AbortSignal.timeout(opts.timeoutMs ?? REQUEST_TIMEOUT_MS);
  const signal = opts.signal ? AbortSignal.any([opts.signal, timeout]) : timeout;

  let res: Response;
  try {
    res = await fetch(path, { method, headers, body, signal, credentials: 'same-origin' });
  } catch (err) {
    // Отмену вызывающим (React Query при уходе с экрана) пробрасываем как есть.
    if (opts.signal?.aborted) throw err;
    reportConnection(false);
    throw new NetworkError(timeout.aborted ? 'timeout' : 'offline');
  }

  // 502–504 от nginx — без JSON; 503 с {error} — ответ самого garage («Бот не настроен»).
  const gateway = GATEWAY_STATUSES.has(res.status);
  const data = gateway ? await readJson(res.clone()) : await readJson(res);
  if (gateway && !(res.status === 503 && hasErrorText(data))) {
    reportConnection(false);
    throw new NetworkError(res.status === 504 ? 'timeout' : 'offline');
  }

  // Ответ без JSON-тела ошибки при 5xx — это не garage, а прокси перед ним
  // (например, Vite при остановленном сервере): тоже «нет связи».
  if (!res.ok && res.status >= 500 && !hasErrorText(data)) {
    reportConnection(false);
    throw new NetworkError();
  }

  reportConnection(true);

  if (!res.ok) throw toApiError(res, data);
  return data as T;
}

async function readJson(res: Response): Promise<unknown> {
  if (res.status === 204) return undefined;
  const text = await res.text().catch(() => '');
  if (text === '') return undefined;
  try {
    return JSON.parse(text) as unknown;
  } catch {
    return undefined;
  }
}

type ErrorBody = { error: string; field?: unknown; retry_after?: unknown; available?: unknown };

function hasErrorText(data: unknown): data is ErrorBody {
  return typeof data === 'object' && data !== null && typeof (data as ErrorBody).error === 'string';
}

function toApiError(res: Response, data: unknown): ApiError {
  const body = hasErrorText(data) ? data : undefined;
  const message = body?.error || `Сервер ответил ${res.status}`;
  const field = typeof body?.field === 'string' ? body.field : undefined;
  let retryAfter = typeof body?.retry_after === 'number' ? body.retry_after : undefined;
  if (retryAfter === undefined) {
    const header = Number(res.headers.get('Retry-After'));
    if (Number.isFinite(header) && header > 0) retryAfter = header;
  }
  const available = typeof body?.available === 'number' ? body.available : undefined;
  return new ApiError(res.status, message, field, retryAfter, available);
}
