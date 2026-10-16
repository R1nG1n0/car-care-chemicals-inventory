// В node_modules бывают исходники на Go (например, flatted). Отдельный
// go.mod делает каталог чужим модулем, и `go test ./...`, `go vet ./...`
// панели в него не заходят.
import { writeFileSync, existsSync } from 'node:fs';

if (existsSync('node_modules')) {
  writeFileSync('node_modules/go.mod', 'module node_modules\n');
}
