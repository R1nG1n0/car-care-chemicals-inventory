// Состояние входа на React Query. Ключ ['me']: Session — вошли, null — нет.
// 401 из любого запроса обнуляет сессию — защищённые экраны уводят на вход.
import { useSyncExternalStore } from 'react';
import { MutationCache, QueryCache, QueryClient, queryOptions, useQuery } from '@tanstack/react-query';
import { api, isApiError, isNetworkError, type Session } from '../api';
import { isOnline, subscribeConnection } from '../api/connection';
import { installOffline } from './offline';
import { invalidateStock } from './queries';

export const meKey = ['me'] as const;

export const meQuery = queryOptions({
  queryKey: meKey,
  queryFn: async ({ signal }): Promise<Session | null> => {
    try {
      return await api.me(signal);
    } catch (err) {
      if (isApiError(err, 401)) return null;
      throw err;
    }
  },
  staleTime: 60_000,
});

export function createQueryClient(): QueryClient {
  const onError = (err: unknown) => {
    if (!isApiError(err, 401)) return;
    if (client.getQueryData(meKey) !== null) clearSession(client);
  };
  // Ответ на изменение не пришёл (тайм-аут): на сервере оно могло выполниться.
  // Повторно не отправляем, а перечитываем данные — экран покажет, что сохранилось.
  const onMutationError = (err: unknown) => {
    onError(err);
    if (isNetworkError(err) && err.maybeSaved) void invalidateStock(client);
  };
  const client: QueryClient = new QueryClient({
    queryCache: new QueryCache({ onError }),
    mutationCache: new MutationCache({ onError: onMutationError }),
    defaultOptions: {
      queries: {
        // Не ждать navigator.onLine: связь определяется ответом сервера через VPN.
        networkMode: 'always',
        retry: (count, err) => isNetworkError(err) && count < 1,
        retryDelay: 1000,
      },
      mutations: { networkMode: 'always', retry: false },
    },
  });
  // Сохранённые данные и очередь без связи (ФТ-12)
  installOffline(client);
  return client;
}

export function useSession() {
  return useQuery(meQuery);
}

/** Вошли или приняли приглашение: запоминаем сессию без лишнего запроса. */
export function setSession(client: QueryClient, s: Session): void {
  client.setQueryData(meKey, s);
}

/** Вышли: забываем все данные прошлого пользователя. */
export function clearSession(client: QueryClient): void {
  client.removeQueries({ predicate: (q) => q.queryKey[0] !== meKey[0] });
  client.setQueryData(meKey, null);
}

export function useOnline(): boolean {
  return useSyncExternalStore(subscribeConnection, isOnline);
}
