// Импорт файла экспорта (ФТ-10.3.3): POST /api/import. Скачивание — обычными ссылками.
import { request } from './client';
import type { TransferCounts } from '../lib/transfer';

/** Файл до 100 МБ по мобильной сети грузится долго; чуть дольше срока сервера (internal/web/handlers_transfer.go). */
const IMPORT_TIMEOUT_MS = 12 * 60_000;

export const transfer = {
  /** text — содержимое файла как есть. */
  importFile: async (text: string) =>
    (await request<{ imported: TransferCounts }>('/api/import', { method: 'POST', rawBody: text, timeoutMs: IMPORT_TIMEOUT_MS })).imported,
};
