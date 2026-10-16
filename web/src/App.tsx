import { useEffect, useState } from 'react';

type Health = { status: string; version: string };

type State =
  | { kind: 'loading' }
  | { kind: 'ok'; health: Health }
  | { kind: 'error'; message: string };

// Каркас этапа 0: проверяет, что интерфейс собран, встроен в бинарник
// и достучался до API. Экраны v1.0 появятся на этапе 3.
export function App() {
  const [state, setState] = useState<State>({ kind: 'loading' });

  useEffect(() => {
    const ctrl = new AbortController();
    fetch('/api/health', { signal: ctrl.signal })
      .then((r) => {
        if (!r.ok) throw new Error(`Сервер ответил ${r.status}`);
        return r.json() as Promise<Health>;
      })
      .then((health) => setState({ kind: 'ok', health }))
      .catch((err: unknown) => {
        if (ctrl.signal.aborted) return;
        setState({ kind: 'error', message: err instanceof Error ? err.message : 'Сервер недоступен' });
      });
    return () => ctrl.abort();
  }, []);

  return (
    <main className="app-shell">
      <h1 className="app-logo">
        garage<span className="app-logo__dot">.</span>
      </h1>
      <p className="g-muted">Учёт автохимии и инструментов</p>

      <section className="g-card app-status" aria-live="polite">
        <div className="g-card__strip app-status__strip" />
        <div className="g-card__body">
          <div className="g-label">Сервер</div>
          {state.kind === 'loading' && <p className="app-status__text">Проверяю…</p>}
          {state.kind === 'ok' && (
            <p className="app-status__text">
              Работает · версия <span className="g-num">{state.health.version}</span>
            </p>
          )}
          {state.kind === 'error' && <p className="app-status__text app-status__text--error">{state.message}</p>}
        </div>
      </section>

      <p className="g-muted app-note">Каркас приложения. Экраны остатков, покупок и расхода появятся на этапе 3.</p>
    </main>
  );
}
