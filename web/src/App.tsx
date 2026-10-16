// Корень приложения: React Query, маршруты, состояние входа.
// Пока идёт первый /api/auth/me — подложки; сервер не ответил — «Сервер недоступен»;
// дальше вход или приложение. 401 из любого запроса уводит на вход.
import { QueryClientProvider } from '@tanstack/react-query';
import { useState, type ReactNode } from 'react';
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router';
import { ConnectionBanner } from './app/ConnectionBanner';
import { createQueryClient, useSession } from './app/session';
import { Shell } from './app/Shell';
import { InviteScreen } from './screens/InviteScreen';
import { LoginScreen } from './screens/LoginScreen';
import { BootScreen, OfflineScreen } from './screens/SystemScreens';
import { returnPath } from './lib/nav';

function RequireAuth({ children }: { children: ReactNode }) {
  const { data } = useSession();
  const location = useLocation();
  if (!data) return <Navigate to="/login" replace state={{ from: location.pathname + location.search }} />;
  return children;
}

function GuestOnly({ children }: { children: ReactNode }) {
  const { data } = useSession();
  const location = useLocation();
  if (data) return <Navigate to={returnPath(location.state)} replace />;
  return children;
}

function Root() {
  const me = useSession();
  if (me.isPending) return <BootScreen />;
  if (me.data === undefined) {
    return <OfflineScreen error={me.error} lastAttempt={me.errorUpdatedAt} retrying={me.isFetching} onRetry={() => void me.refetch()} />;
  }
  const app = (
    <RequireAuth>
      <Shell />
    </RequireAuth>
  );
  return (
    <>
      <ConnectionBanner />
      <Routes>
        <Route
          path="/login"
          element={
            <GuestOnly>
              <LoginScreen />
            </GuestOnly>
          }
        />
        <Route path="/invite/:token" element={<InviteScreen />} />
        <Route path="/more" element={<Navigate to="/profile" replace />} />
        {/* Общий макет: Shell сам выбирает экран по адресу, чтобы держать
            уходящий и входящий экраны одновременно (анимация .g-view). */}
        <Route element={app}>
          <Route path="/" element={null} />
          <Route path="/shopping" element={null} />
          <Route path="/templates" element={null} />
          <Route path="/profile" element={null} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </>
  );
}

export function App() {
  const [client] = useState(createQueryClient);
  return (
    <QueryClientProvider client={client}>
      <BrowserRouter>
        <Root />
      </BrowserRouter>
    </QueryClientProvider>
  );
}
