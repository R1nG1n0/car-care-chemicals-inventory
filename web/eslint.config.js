import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import reactHooks from 'eslint-plugin-react-hooks';
import { defineConfig } from 'eslint/config';

// Запреты: вставка HTML и выполнение строк как кода (ТБ-6).
const unsafe = [
  { selector: "JSXAttribute[name.name='dangerouslySetInnerHTML']", message: 'Вставка HTML запрещена: выводите данные текстом.' },
  { selector: "MemberExpression[property.name=/^(innerHTML|outerHTML)$/]", message: 'innerHTML/outerHTML запрещены.' },
  { selector: "CallExpression[callee.property.name='insertAdjacentHTML']", message: 'insertAdjacentHTML запрещён.' },
  { selector: "CallExpression[callee.object.name='document'][callee.property.name=/^write(ln)?$/]", message: 'document.write запрещён.' },
  { selector: "CallExpression[callee.property.name='setAttribute'][arguments.0.value='style']", message: 'Атрибут style запрещён политикой style-src.' },
  { selector: "NewExpression[callee.name='Function']", message: 'new Function запрещён.' },
];

export default defineConfig(
  { ignores: ['dist/', 'node_modules/'] },
  js.configs.recommended,
  tseslint.configs.strict,
  reactHooks.configs.flat['recommended-latest'],
  {
    rules: {
      // Индексы массивов в строгом режиме (noUncheckedIndexedAccess) дают
      // T | undefined; там, где длина проверена, «!» честнее заглушек.
      '@typescript-eslint/no-non-null-assertion': 'off',
      'no-eval': 'error',
      'no-implied-eval': 'error',
      'no-new-func': 'error',
      'no-restricted-syntax': ['error', ...unsafe],
    },
  },
  {
    // Шаблон сервис-воркера: глобальные имена воркера
    files: ['src/sw/sw.js'],
    languageOptions: {
      globals: { self: 'readonly', caches: 'readonly', Request: 'readonly', Response: 'readonly', URL: 'readonly', fetch: 'readonly', AbortController: 'readonly', setTimeout: 'readonly', clearTimeout: 'readonly' },
    },
  },
  {
    files: ['scripts/**/*.mjs', '*.config.{js,ts}'],
    languageOptions: { globals: { process: 'readonly', console: 'readonly' } },
  },
);
