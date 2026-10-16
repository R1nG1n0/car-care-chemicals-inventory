// Проверка сборки на совместимость с политикой безопасности (ТБ-6):
// script-src 'self'; style-src 'self' — никаких встроенных скриптов,
// стилей, атрибутов style и внешних адресов в index.html, никакого eval
// и new Function в скриптах.
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';

const fail = (msg) => { console.error('check-dist: ' + msg); process.exit(1); };

const html = readFileSync('dist/index.html', 'utf8');
for (const m of html.matchAll(/<script\b([^>]*)>([\s\S]*?)<\/script>/gi)) {
  if (!/\bsrc=/.test(m[1])) fail('встроенный <script> в index.html');
  if (m[2].trim() !== '') fail('код внутри <script src> в index.html');
}
if (/<style\b/i.test(html)) fail('встроенный <style> в index.html');
if (/\sstyle=/i.test(html)) fail('атрибут style в index.html');
if (/\son[a-z]+=/i.test(html)) fail('обработчик события в атрибуте в index.html');
if (/(src|href)=["']?(https?:)?\/\//i.test(html)) fail('внешний адрес в index.html');

const files = [];
const walk = (dir) => {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) walk(p); else files.push(p);
  }
};
walk('dist');

let total = 0;
for (const f of files) {
  const size = statSync(f).size;
  total += size;
  if (f.endsWith('.js')) {
    const js = readFileSync(f, 'utf8');
    if (/\beval\(/.test(js)) fail(`eval в ${f}`);
    if (/new Function\(/.test(js)) fail(`new Function в ${f}`);
  }
  if (f.endsWith('.css') && /url\(\s*["']?(https?:)?\/\//.test(readFileSync(f, 'utf8'))) fail(`внешний адрес в ${f}`);
}
for (const f of files.sort()) console.log(`  ${f.padEnd(40)} ${(statSync(f).size / 1024).toFixed(1).padStart(8)} КБ`);
console.log(`check-dist: ок, ${files.length} файлов, ${(total / 1024).toFixed(1)} КБ`);
