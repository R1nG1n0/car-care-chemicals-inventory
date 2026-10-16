// Как предложить установку приложения (И-2.1):
// - Android/Chrome прислал beforeinstallprompt — кнопка «Установить приложение»;
// - iPhone/iPad в Safari вне режима standalone — подсказка «Поделиться → На экран „Домой“»;
// - уже установлено или браузер не умеет — ничего.

export type InstallMode = 'prompt' | 'ios' | 'none';

export type InstallEnv = {
  userAgent: string;
  /** navigator.maxTouchPoints: iPadOS выдаёт себя за Mac, но с сенсорным экраном. */
  maxTouchPoints: number;
  /** display-mode: standalone или navigator.standalone (iOS). */
  standalone: boolean;
  /** Сохранено событие beforeinstallprompt. */
  hasPrompt: boolean;
};

export function isIOS(ua: string, maxTouchPoints: number): boolean {
  if (/iPhone|iPad|iPod/.test(ua)) return true;
  return /Macintosh/.test(ua) && maxTouchPoints > 1;
}

/** Safari, а не Chrome/Firefox/Edge/Opera/Яндекс на iOS — у них свои меню. */
export function isIOSSafari(ua: string): boolean {
  return /Safari\//.test(ua) && !/CriOS|FxiOS|EdgiOS|OPiOS|OPT\/|YaBrowser|GSA\//.test(ua);
}

export function installMode(env: InstallEnv): InstallMode {
  if (env.standalone) return 'none';
  if (env.hasPrompt) return 'prompt';
  if (isIOS(env.userAgent, env.maxTouchPoints) && isIOSSafari(env.userAgent)) return 'ios';
  return 'none';
}
