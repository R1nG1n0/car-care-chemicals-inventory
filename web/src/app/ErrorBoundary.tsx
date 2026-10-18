// Граница ошибок экрана: исключение при отрисовке не даёт белый сайт, а показывает
// «Что-то пошло не так» с «Повторить» (перерисовать экран) и «Перезагрузить».
// resetKey — адрес: переход на другой экран сбрасывает ошибку сам.
import { IconAlertTriangle } from '@tabler/icons-react';
import { Component, type ErrorInfo, type ReactNode } from 'react';
import { Empty } from '../components/ui';

type Props = { resetKey: string; children: ReactNode };
type State = { error: Error | null; key: string };

export class ErrorBoundary extends Component<Props, State> {
  override state: State = { error: null, key: this.props.resetKey };

  static getDerivedStateFromError(error: unknown): Partial<State> {
    return { error: error instanceof Error ? error : new Error(String(error)) };
  }

  static getDerivedStateFromProps(props: Props, state: State): Partial<State> | null {
    // Другой адрес — другой экран: прежняя ошибка к нему не относится
    return props.resetKey !== state.key ? { key: props.resetKey, error: null } : null;
  }

  override componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error('Ошибка экрана', error, info.componentStack);
  }

  override render(): ReactNode {
    if (!this.state.error) return this.props.children;
    return (
      <Empty
        id="app-crash"
        icon={IconAlertTriangle}
        error
        title="Что-то пошло не так"
        text="Экран не отрисовался. Данные на сервере не пострадали — повторите или перезагрузите страницу."
        actions={
          <>
            <button type="button" className="g-btn g-btn--block g-press" onClick={() => this.setState({ error: null })}>
              Повторить
            </button>
            <button type="button" className="g-btn g-btn--ghost g-btn--block" onClick={() => window.location.reload()}>
              Перезагрузить
            </button>
          </>
        }
      />
    );
  }
}
