// Чистит web/dist перед сборкой, оставляя .keep: без него go:embed не
// соберётся на машине, где интерфейс ещё не собирали.
import { readdirSync, rmSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';

mkdirSync('dist', { recursive: true });
for (const name of readdirSync('dist')) {
  if (name !== '.keep') rmSync(join('dist', name), { recursive: true, force: true });
}
