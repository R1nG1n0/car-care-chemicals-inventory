import { describe, expect, it } from 'vitest';
import { installMode, type InstallEnv } from './install';

const UA = {
  iphoneSafari: 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Mobile/15E148 Safari/604.1',
  iphoneChrome: 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/140.0.0.0 Mobile/15E148 Safari/604.1',
  ipadOS: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Safari/605.1.15',
  android: 'Mozilla/5.0 (Linux; Android 15; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Mobile Safari/537.36',
  macSafari: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Safari/605.1.15',
};

const env = (over: Partial<InstallEnv>): InstallEnv => ({ userAgent: UA.android, maxTouchPoints: 0, standalone: false, hasPrompt: false, ...over });

describe('installMode', () => {
  it('Android с beforeinstallprompt — кнопка', () => expect(installMode(env({ hasPrompt: true, maxTouchPoints: 5 }))).toBe('prompt'));
  it('Android без события — ничего', () => expect(installMode(env({ maxTouchPoints: 5 }))).toBe('none'));
  it('iPhone в Safari — подсказка', () => expect(installMode(env({ userAgent: UA.iphoneSafari, maxTouchPoints: 5 }))).toBe('ios'));
  it('iPhone в Chrome — ничего', () => expect(installMode(env({ userAgent: UA.iphoneChrome, maxTouchPoints: 5 }))).toBe('none'));
  it('iPad с «настольным» UA — подсказка', () => expect(installMode(env({ userAgent: UA.ipadOS, maxTouchPoints: 5 }))).toBe('ios'));
  it('Safari на Mac — ничего', () => expect(installMode(env({ userAgent: UA.macSafari, maxTouchPoints: 0 }))).toBe('none'));
  it('уже установлено — ничего, даже с событием', () => {
    expect(installMode(env({ userAgent: UA.iphoneSafari, maxTouchPoints: 5, standalone: true }))).toBe('none');
    expect(installMode(env({ hasPrompt: true, standalone: true }))).toBe('none');
  });
});
