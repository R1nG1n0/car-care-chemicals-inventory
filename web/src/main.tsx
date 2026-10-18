import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';

// Шрифты поставляются вместе с приложением (И-8): Vite кладёт файлы в assets/.
// Только латиница и кириллица — остальные наборы символов не нужны.
import '@fontsource/manrope/latin-500.css';
import '@fontsource/manrope/cyrillic-500.css';
import '@fontsource/manrope/latin-600.css';
import '@fontsource/manrope/cyrillic-600.css';
import '@fontsource/manrope/latin-700.css';
import '@fontsource/manrope/cyrillic-700.css';
import '@fontsource/manrope/latin-800.css';
import '@fontsource/manrope/cyrillic-800.css';
import '@fontsource/unbounded/latin-800.css';
import '@fontsource/unbounded/cyrillic-800.css';

import './styles/tokens.css';
import './styles/components.css';
import './styles/states.css';
import './styles/motion.css';
import './styles/app.css';
import './styles/stock.css';
import './styles/shopping.css';
import './styles/scanner.css';
import { App } from './App';
import { listenInstallPrompt } from './app/install';
import { syncThemeColor } from './app/themeColor';

// До первого рендера: beforeinstallprompt приходит рано (И-2.1),
// цвет строки состояния — по текущей теме (И-2.2).
listenInstallPrompt();
syncThemeColor();

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
