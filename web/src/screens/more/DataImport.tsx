// Загрузка из файла (ФТ-10.3.3): выбор файла, предпросмотр счётчиков, подтверждение.
// Только в пустой склад; решает сервер, кнопка недоступна заранее по списку позиций.
import { IconAlertTriangle, IconUpload } from '@tabler/icons-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useRef, useState, type ChangeEvent } from 'react';
import { inventory, isApiError, transfer } from '../../api';
import { keys } from '../../app/queries';
import { useToast } from '../../app/toast';
import { busyProps, Ico } from '../../components/ui';
import { countLines, exportedLabel, fileSizeError, importedText, isWarehouseEmpty, parseExportFile, type ExportSummary } from '../../lib/transfer';

type Picked = { name: string; text: string; summary: ExportSummary };

export function DataImport() {
  const client = useQueryClient();
  const toast = useToast();
  const input = useRef<HTMLInputElement>(null);
  const [picked, setPicked] = useState<Picked | null>(null);
  const [error, setError] = useState('');

  const active = useQuery({ queryKey: keys.products(), queryFn: ({ signal }) => inventory.products({}, signal) });
  const archived = useQuery({ queryKey: keys.products({ archived: true }), queryFn: ({ signal }) => inventory.products({ archived: true }, signal) });
  const checked = active.data !== undefined && archived.data !== undefined;
  const empty = checked && isWarehouseEmpty(active.data.length, archived.data.length);

  const run = useMutation({
    mutationFn: (text: string) => transfer.importFile(text),
    onSuccess: async (counts) => {
      setPicked(null);
      setError('');
      // Импорт меняет всё: остатки, справочники, шаблоны, шаги кнопок
      await client.invalidateQueries();
      toast.show({ text: `Данные загружены: ${importedText(counts)}`, duration: 5000 });
    },
    onError: (err) => setError(isApiError(err) ? err.message : 'Не удалось загрузить файл. Проверьте связь и повторите.'),
  });

  const choose = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = ''; // тот же файл можно выбрать повторно
    if (!file) return;
    setPicked(null);
    setError('');
    const tooBig = fileSizeError(file.size);
    if (tooBig) return setError(tooBig);
    let text: string;
    try {
      text = await file.text();
    } catch {
      return setError('Не удалось прочитать файл.');
    }
    const r = parseExportFile(text);
    if (!r.ok) return setError(r.error);
    setPicked({ name: file.name, text, summary: r.summary });
  };

  const date = picked ? exportedLabel(picked.summary.exportedAt) : null;
  const lines = picked ? countLines(picked.summary.counts) : [];

  return (
    <section className="g-section" aria-labelledby="data-up">
      <span className="g-label" id="data-up">
        Загрузить из файла
      </span>
      <p className="g-muted">Перенос и восстановление: файл, скачанный кнопкой «Скачать всё с фото» или «Скачать без фото». Файл — до 100 МБ; загрузить можно только в пустой склад.</p>

      {checked && !empty && (
        <p className="g-banner" id="data-busy">
          <Ico icon={IconAlertTriangle} />
          <span className="g-banner__text">
            В вашем складе уже есть позиции, поэтому загрузка недоступна. Чтобы перенести данные, войдите под новым пользователем и загрузите файл там.
          </span>
        </p>
      )}
      {(active.isError || archived.isError) && !checked && (
        <p className="g-field__error" role="alert">
          Не удалось проверить склад. Откройте экран ещё раз, когда появится связь.
        </p>
      )}

      <input ref={input} id="data-file" type="file" accept=".json,application/json" hidden onChange={(e) => void choose(e)} />
      {!picked && (
        <button
          type="button"
          className="g-btn g-btn--block g-press"
          disabled={!empty}
          aria-disabled={!empty || undefined}
          aria-describedby={checked && !empty ? 'data-busy' : undefined}
          onClick={() => input.current?.click()}
        >
          <Ico icon={IconUpload} /> {checked || active.isError || archived.isError ? 'Выбрать файл' : 'Проверяю склад…'}
        </button>
      )}

      {picked && (
        <div className="g-invite" role="group" aria-labelledby="data-preview">
          <b id="data-preview">Загрузить этот файл?</b>
          <span className="g-invite__meta">
            {picked.name}
            {picked.summary.login && ` · данные пользователя ${picked.summary.login}`}
            {date && ` · от ${date}`}
          </span>
          {lines.length > 0 ? (
            <ul className="data-counts" aria-label="Что в файле">
              {lines.map((l) => (
                <li key={l}>{l}</li>
              ))}
            </ul>
          ) : (
            <span className="g-invite__meta">В файле нет записей.</span>
          )}
          <span className="g-invite__meta">Одинаковые по названию категории, места и магазины объединятся. Отменить загрузку нельзя.</span>
          <div className="g-invite__actions">
            <button type="button" className="g-btn g-btn--sm g-press" autoFocus onClick={() => !run.isPending && run.mutate(picked.text)} {...busyProps(run.isPending)}>
              {run.isPending ? 'Загружаю…' : 'Загрузить'}
            </button>
            <button type="button" className="g-btn g-btn--sm g-btn--ghost g-press" disabled={run.isPending} onClick={() => setPicked(null)}>
              Отмена
            </button>
          </div>
        </div>
      )}
      {error && (
        <p className="g-field__error" role="alert" id="data-error">
          {error}
        </p>
      )}
    </section>
  );
}
