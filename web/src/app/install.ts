// Установка приложения (И-2.1). Chrome присылает beforeinstallprompt один раз
// и рано — поэтому слушатель ставится при запуске (main.tsx), а событие хранится
// здесь до нажатия кнопки в профиле.
import { useSyncExternalStore } from 'react';
import { installMode, type InstallMode } from '../lib/install';

type BeforeInstallPromptEvent = Event & {
  prompt: () => Promise<void>;
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>;
};

let deferred: BeforeInstallPromptEvent | null = null;
let installed = false;
let version = 0;
const listeners = new Set<() => void>();
const notify = () => {
  version++;
  for (const l of listeners) l();
};

const standaloneQuery = () => window.matchMedia('(display-mode: standalone)');

function isStandalone(): boolean {
  const nav = navigator as Navigator & { standalone?: boolean };
  return installed || nav.standalone === true || standaloneQuery().matches;
}

export function listenInstallPrompt(): void {
  window.addEventListener('beforeinstallprompt', (e) => {
    // Без preventDefault Chrome покажет свою плашку сам — нам нужна кнопка в профиле.
    e.preventDefault();
    deferred = e as BeforeInstallPromptEvent;
    notify();
  });
  window.addEventListener('appinstalled', () => {
    deferred = null;
    installed = true;
    notify();
  });
  standaloneQuery().addEventListener('change', notify);
}

function subscribe(l: () => void) {
  listeners.add(l);
  return () => listeners.delete(l);
}

export function useInstallMode(): InstallMode {
  // Снимок — номер версии: сам режим зависит и от matchMedia, и от события.
  useSyncExternalStore(subscribe, () => version);
  return installMode({
    userAgent: navigator.userAgent,
    maxTouchPoints: navigator.maxTouchPoints,
    standalone: isStandalone(),
    hasPrompt: deferred !== null,
  });
}

/** Показать системный диалог установки. Событие одноразовое. */
export async function promptInstall(): Promise<boolean> {
  const e = deferred;
  if (!e) return false;
  deferred = null;
  await e.prompt();
  const { outcome } = await e.userChoice;
  notify();
  return outcome === 'accepted';
}
