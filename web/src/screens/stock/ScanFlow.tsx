// Сканирование на главной (ФТ-9.2, 9.4, 9.5): сканер → поиск позиций с точно
// таким кодом (GET /api/products?q= — действующие и архивные, сравнение на
// клиенте, lib/barcode) → одна — карточка позиции, несколько — выбор, ни одной —
// шторка «Код … не найден» с «Создать позицию» и «Привязать к существующей».
// Привязка: PATCH /api/products/{id} {barcode}; чужой код у позиции — спросить;
// после — уведомление «Код привязан к …» с «Отменить» (вернуть прежний код).
import { IconCirclePlus, IconLink, IconScan } from '@tabler/icons-react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router';
import { errorText, inventory, type ProductSummary } from '../../api';
import { invalidateStock } from '../../app/queries';
import { useToast } from '../../app/toast';
import { applyProducts } from '../../app/useRevert';
import { ProductPicker } from '../../components/ProductPicker';
import { catClassOf, useStockRefs } from '../../components/refs';
import { Scanner } from '../../components/Scanner';
import { Sheet } from '../../components/Sheet';
import { busyProps, Ico } from '../../components/ui';
import { bindPlan, findExact, scanOutcome, searchKey } from '../../lib/barcode';
import { qtyText } from '../../lib/stock';

type Step =
  | { k: 'scan'; busy: boolean }
  | { k: 'choose'; code: string; items: ProductSummary[] }
  | { k: 'none'; code: string }
  | { k: 'bind'; code: string }
  | { k: 'confirm'; code: string; product: ProductSummary }
  | { k: 'fail'; code: string; error: string; busy: boolean };

type SheetStep = Exclude<Step, { k: 'scan' }>;
type Bind = { product: ProductSummary; code: string };

export function ScanFlow({ open, onClose }: { open: boolean; onClose: () => void }) {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const toast = useToast();
  const refs = useStockRefs();
  const [step, setStep] = useState<Step>({ k: 'scan', busy: false });
  const [session, setSession] = useState(0);
  const [wasOpen, setWasOpen] = useState(open);
  // Что показывает шторка: при уходе к сканеру она доигрывает закрытие с прежним текстом
  const [shown, setShown] = useState<SheetStep | null>(null);
  const seq = useRef(0);
  if (step.k !== 'scan' && step !== shown) setShown(step);

  // Каждое открытие — заново со сканера
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) {
      setStep({ k: 'scan', busy: false });
      setSession((s) => s + 1);
    }
  }

  // Закрыли, пока шёл поиск, — ответ уже не нужен
  useEffect(() => {
    if (!open) seq.current += 1;
  }, [open]);

  const go = (path: string) => {
    onClose();
    navigate(path);
  };

  const lookup = async (code: string, viaScanner: boolean) => {
    const my = ++seq.current;
    setStep(viaScanner ? { k: 'scan', busy: true } : { k: 'fail', code, error: '', busy: true });
    try {
      const q = searchKey(code);
      const [active, archived] = await Promise.all([inventory.products({ q }), inventory.products({ q, archived: true })]);
      if (my !== seq.current) return;
      const out = scanOutcome(findExact([...active, ...archived], code));
      if (out.kind === 'open') go(`/items/${out.product.id}`);
      else if (out.kind === 'choose') setStep({ k: 'choose', code, items: out.products });
      else setStep({ k: 'none', code });
    } catch (err) {
      if (my === seq.current) setStep({ k: 'fail', code, error: errorText(err), busy: false });
    }
  };

  const rescan = () => {
    setSession((s) => s + 1);
    setStep({ k: 'scan', busy: false });
  };

  // Откат только если код всё ещё тот, что привязали: за 10 секунд его могли изменить
  const undo = (id: number, bound: string, prev: string) => {
    inventory
      .product(id)
      .then(({ product: cur }) => {
        if (cur.barcode !== bound) {
          toast.show({ text: 'Код уже изменён — отмена не нужна' });
          return undefined;
        }
        return inventory.updateProduct(id, { barcode: prev });
      })
      .then((r) => {
        if (!r) return;
        applyProducts(qc, [r]);
        void invalidateStock(qc);
        toast.show({ text: prev ? `Прежний код возвращён: ${r.name}` : `Код отвязан от ${r.name}` });
      })
      .catch((err: unknown) => toast.show({ text: `Не отменено: ${errorText(err)}` }));
  };

  const bind = useMutation({
    mutationFn: ({ product, code }: Bind) => inventory.updateProduct(product.id, { barcode: code }),
    onSuccess: (r, { product }) => {
      applyProducts(qc, [r]);
      void invalidateStock(qc);
      go(`/items/${r.id}`);
      toast.show({ text: `Код привязан к ${r.name}`, action: { label: 'Отменить', onClick: () => undo(r.id, r.barcode, product.barcode) } });
    },
  });

  const pick = (p: ProductSummary, code: string) => {
    if (bind.isPending) return;
    bind.reset();
    const plan = bindPlan(p, code);
    if (plan === 'already') go(`/items/${p.id}`);
    else if (plan === 'replace') setStep({ k: 'confirm', code, product: p });
    else bind.mutate({ product: p, code });
  };

  // Выбор позиции открывается внутри той же шторки — фокус в поиск вручную
  useEffect(() => {
    if (step.k === 'bind') document.getElementById('scan-bind')?.focus({ preventScroll: true });
  }, [step.k]);

  const sheetOpen = open && step.k !== 'scan';
  const bindError = bind.isError ? <p className="g-field__error" role="alert">{errorText(bind.error)}</p> : null;

  let title = '';
  let body = null;
  const view = shown;
  if (view === null) {
    // Шторка ещё не открывалась
  } else if (view.k === 'none') {
    title = `Код ${view.code} не найден`;
    const code = view.code;
    body = (
      <>
        <p className="g-muted">Создайте позицию с этим штрихкодом или привяжите код к уже заведённой — дальше сканер будет находить её сразу.</p>
        <button type="button" className="g-btn g-btn--cta g-btn--block g-press" data-autofocus onClick={() => go(`/items/new?barcode=${encodeURIComponent(code)}`)}>
          <Ico icon={IconCirclePlus} /> Создать позицию
        </button>
        <button type="button" className="g-btn g-btn--block g-press" onClick={() => setStep({ k: 'bind', code })}>
          <Ico icon={IconLink} /> Привязать к существующей
        </button>
        <RescanButton onClick={rescan} />
      </>
    );
  } else if (view.k === 'bind') {
    title = `Привязать код ${view.code}`;
    const code = view.code;
    body = (
      <>
        <ProductPicker id="scan-bind" label="К какой позиции" onChange={(p) => pick(p, code)} />
        {bind.isPending && <p className="g-muted" aria-busy="true">Привязываю код…</p>}
        {bindError}
        <button type="button" className="g-btn g-btn--block g-btn--ghost" onClick={() => setStep({ k: 'none', code })}>
          Назад
        </button>
      </>
    );
  } else if (view.k === 'confirm') {
    const { product: p, code } = view;
    title = 'Заменить штрихкод?';
    body = (
      <>
        <p>
          У «{p.name}» уже есть код <b className="scan-code">{p.barcode}</b>. Записать вместо него <b className="scan-code">{code}</b>?
        </p>
        {bindError}
        <button type="button" className="g-btn g-btn--cta g-btn--block g-press" {...busyProps(bind.isPending)} onClick={() => !bind.isPending && bind.mutate({ product: p, code })}>
          {bind.isPending ? 'Заменяю…' : 'Заменить код'}
        </button>
        <button type="button" className="g-btn g-btn--block g-btn--ghost" onClick={() => setStep({ k: 'bind', code })}>
          Выбрать другую позицию
        </button>
      </>
    );
  } else if (view.k === 'choose') {
    title = `Код ${view.code}`;
    body = (
      <>
        <p className="g-muted">Этот код есть у нескольких позиций — выберите нужную.</p>
        <div className="g-list">
          {view.items.map((p) => (
            <button key={p.id} type="button" className={`g-row g-row--link app-row-btn ${catClassOf(refs.categories, p)}`} onClick={() => go(`/items/${p.id}`)}>
              <span className="g-cat-sq" />
              <span className="g-row__main">
                <span className="g-row__title stock-block">{p.name}</span>
                <span className="g-row__sub stock-block">{[p.brand, p.stock.total > 0 ? qtyText(p.stock.total, p.unit) : 'нет в наличии'].filter(Boolean).join(' · ')}</span>
              </span>
              {p.archived && <span className="g-badge">в архиве</span>}
            </button>
          ))}
        </div>
        <RescanButton onClick={rescan} />
      </>
    );
  } else if (view.k === 'fail') {
    const code = view.code;
    title = 'Код не проверен';
    body = (
      <>
        <p className="g-muted" role={view.busy ? undefined : 'alert'}>
          {view.busy ? `Ищу код ${code}…` : `Не получилось найти код ${code}: ${view.error}`}
        </p>
        <button type="button" className="g-btn g-btn--cta g-btn--block g-press" {...busyProps(view.busy)} onClick={() => !view.busy && void lookup(code, false)}>
          Повторить
        </button>
        <RescanButton onClick={rescan} />
      </>
    );
  }

  return (
    <>
      <Scanner
        key={session}
        open={open && step.k === 'scan'}
        onClose={onClose}
        onCode={(code) => step.k === 'scan' && !step.busy && void lookup(code, true)}
        busyText={step.k === 'scan' && step.busy ? 'Ищу позицию…' : null}
      />
      <Sheet open={sheetOpen} onClose={onClose} title={title || 'Сканер'}>
        {body}
      </Sheet>
    </>
  );
}

function RescanButton({ onClick }: { onClick: () => void }) {
  return (
    <button type="button" className="g-btn g-btn--block g-btn--ghost" onClick={onClick}>
      <Ico icon={IconScan} /> Сканировать снова
    </button>
  );
}
