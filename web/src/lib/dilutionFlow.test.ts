// Раствор в очереди без связи, оценке остатка, отправке, журнале, шаблонах и форме позиции (ФТ-17).
import { describe, expect, it, vi } from 'vitest';
import type { ProductSummary, Recipe } from '../api';
import { draftsOf } from './dilution';
import { movementTitle, revertQuestion, solutionTotals } from './movements';
import { estimateByKey, pendingDeltas } from './offlineEstimate';
import { makeApplyOp, makeConsumeOp, OfflineQueue, opQtyText, parseQueue, type ConsumeOp } from './offlineQueue';
import { applyLinesOf, consumeBodyOf, flushQueue, type SendApi } from './offlineSync';
import { validateProduct, type ProductFields } from './product';
import { applyItems, draftOfItem, qtyChanged, validateRecipe } from './recipeApply';
import { memoryStorage } from './testStorage';

const norm = (s: string | null) => (s ?? '').replace(/\s/gu, ' ');
const shampoo = { id: 7, name: 'Шампунь', unit: 'ml' as const };
const sol = { volume_ml: 1000, ratio: 10 };

describe('очередь: расход раствором', () => {
  it('qty — посчитанный концентрат, раствор хранится, переданное число игнорируется', () => {
    const op = makeConsumeOp(shampoo, 12345, undefined, 'op-00000001', 100, sol);
    expect(op.qty).toBe(91);
    expect(op.solution).toEqual(sol);
  });
  it('без раствора — как раньше, поля solution нет', () => {
    const op = makeConsumeOp(shampoo, 40, 3, 'op-00000001', 100);
    expect(op).toEqual({ op_id: 'op-00000001', kind: 'consume', occurred_at: 100, status: 'pending', product_id: 7, qty: 40, name: 'Шампунь', unit: 'ml', unit_id: 3 });
    expect('solution' in op).toBe(false);
  });
  it('«Неотправленное» показывает раствор и концентрат', () => {
    expect(norm(opQtyText(makeConsumeOp(shampoo, 0, undefined, 'op-00000001', 100, sol)))).toBe('Раствор 1 000 мл 1:10 (91 мл)');
    expect(norm(opQtyText(makeConsumeOp(shampoo, 40)))).toBe('40 мл');
  });
  it('шаблон: строка раствором и строка количеством', () => {
    const recipe = { id: 1, name: 'Мойка', items: [{ product_id: 7, product: { name: 'Шампунь', unit: 'ml' } }, { product_id: 8, product: { name: 'Воск', unit: 'ml' } }] } as unknown as Recipe;
    const op = makeApplyOp(recipe, [{ product_id: 7, qty: 0, solution: sol }, { product_id: 8, qty: 30 }], 'op-00000002', 100);
    expect(op.items).toEqual([
      { product_id: 7, qty: 91, solution: sol, name: 'Шампунь', unit: 'ml' },
      { product_id: 8, qty: 30, name: 'Воск', unit: 'ml' },
    ]);
    expect(norm(opQtyText(op))).toBe('Шампунь Раствор 1 000 мл 1:10 (91 мл), Воск 30 мл');
  });
  it('записи старого вида (без solution) читаются, повреждённый раствор отбрасывает запись', () => {
    const old = { op_id: 'a', kind: 'consume', occurred_at: 1, status: 'pending', product_id: 1, qty: 5, name: 'X', unit: 'ml' };
    const withSol = { ...old, op_id: 'b', solution: sol };
    const bad = { ...old, op_id: 'c', solution: { volume_ml: 0, ratio: 10 } };
    const ops = parseQueue(JSON.stringify({ v: 1, ops: [old, withSol, bad] }));
    expect(ops.map((o) => o.op_id)).toEqual(['a', 'b']);
  });
  it('исправление записи с ошибкой числом снимает раствор — спишется ровно столько мл', () => {
    const q = new OfflineQueue(1, memoryStorage());
    q.add(makeConsumeOp(shampoo, 0, undefined, 'op-00000001', 100, sol));
    q.markError('op-00000001', 'Не хватает', 50);
    expect(q.setQty('op-00000001', 50)).toBe(true);
    const op = q.get('op-00000001') as ConsumeOp;
    expect(op.qty).toBe(50);
    expect('solution' in op).toBe(false);
    expect(op.status).toBe('pending');
  });
});

describe('оценка остатка без связи', () => {
  it('вычитается концентрат раствора', () => {
    const ops = [makeConsumeOp(shampoo, 0, undefined, 'op-00000001', 100, sol), makeConsumeOp(shampoo, 40, undefined, 'op-00000002', 101)];
    const d = pendingDeltas(ops);
    expect(d.get(7)).toBe(131);
    const p = { id: 7, unit: 'ml', min_stock: null, low: false, stock: { total: 1000, open_qty: 400, open_capacity: 1000, sealed_count: 1, units_count: 2 } } as unknown as ProductSummary;
    const out = estimateByKey(['products'], [p], d) as ProductSummary[];
    expect(out[0]!.stock.total).toBe(869);
    expect(out[0]!.stock.open_qty).toBe(269);
  });
});

describe('тело запроса', () => {
  it('расход раствором — solution без qty', () => {
    const op = makeConsumeOp(shampoo, 0, 3, 'op-00000001', 100, sol);
    expect(consumeBodyOf(op)).toEqual({ product_id: 7, solution: sol, unit_id: 3, client_op_id: 'op-00000001', occurred_at: 100 });
    expect('qty' in consumeBodyOf(op)).toBe(false);
  });
  it('обычный расход — qty без solution', () => {
    const body = consumeBodyOf(makeConsumeOp(shampoo, 40, undefined, 'op-00000001', 100));
    expect(body).toEqual({ product_id: 7, qty: 40, client_op_id: 'op-00000001', occurred_at: 100 });
  });
  it('строки применения: раствор или количество, не оба', () => {
    expect(
      applyLinesOf([
        { product_id: 7, qty: 91, solution: sol, name: 'А', unit: 'ml' },
        { product_id: 8, qty: 30, name: 'Б', unit: 'ml' },
      ]),
    ).toEqual([{ product_id: 7, solution: sol }, { product_id: 8, qty: 30 }]);
  });
  it('очередь отправляет раствор на сервер', async () => {
    const q = new OfflineQueue(1, memoryStorage());
    q.add(makeConsumeOp(shampoo, 0, undefined, 'op-00000001', 100, sol));
    q.add(makeConsumeOp(shampoo, 40, undefined, 'op-00000002', 101));
    const consume = vi.fn().mockResolvedValue({ movements: [], product: {}, opened_unit_ids: [], finished_unit_ids: [], cost: null });
    const res = await flushQueue(q, { consume, applyRecipe: vi.fn() } as unknown as SendApi);
    expect(res.sent).toBe(2);
    expect(consume.mock.calls.map((c) => c[0])).toEqual([
      { product_id: 7, solution: sol, client_op_id: 'op-00000001', occurred_at: 100 },
      { product_id: 7, qty: 40, client_op_id: 'op-00000002', occurred_at: 101 },
    ]);
  });
});

describe('журнал', () => {
  it('расход раствором: «Расход · раствор 1 000 мл 1:10 → 91 мл»', () => {
    const m = { type: 'consume' as const, qty_delta: -91, reason: '', solution_ml: 1000, solution_ratio: 10 };
    expect(norm(movementTitle(m, 'ml'))).toBe('Расход · раствор 1 000 мл 1:10 → 91 мл');
    expect(norm(revertQuestion(m, 'ml'))).toBe('Отменить: расход · раствор 1 000 мл 1:10 → 91 мл? Остаток пересчитается.');
  });
  it('обычный расход и расход без полей раствора — как прежде', () => {
    expect(norm(movementTitle({ type: 'consume', qty_delta: -40, reason: '', solution_ml: null, solution_ratio: null }, 'ml'))).toBe('Расход · 40 мл');
    expect(norm(movementTitle({ type: 'consume', qty_delta: -40, reason: '' }, 'ml'))).toBe('Расход · 40 мл');
  });
  it('из двух бутылок: общий концентрат по группе запроса', () => {
    const rows = [
      { type: 'consume' as const, qty_delta: -30, batch_id: 5, product_id: 7, solution_ml: 1000 },
      { type: 'consume' as const, qty_delta: -61, batch_id: 5, product_id: 7, solution_ml: 1000 },
      { type: 'consume' as const, qty_delta: -10, batch_id: 6, product_id: 7, solution_ml: null },
    ];
    const t = solutionTotals(rows);
    expect(t.get(5)).toBe(91);
    expect(t.has(6)).toBe(false);
    const m = { type: 'consume' as const, qty_delta: -30, reason: '', solution_ml: 1000, solution_ratio: 10 };
    expect(norm(movementTitle(m, 'ml', t.get(5)))).toBe('Расход · раствор 1 000 мл 1:10 → 91 мл');
  });
});

describe('шаблон', () => {
  it('строка с сервера → форма: раствор разворачивается в объём и пропорцию', () => {
    expect(draftOfItem(3, { product_id: 7, qty: 91, solution: sol })).toEqual({ key: 3, product_id: 7, qty: '91', mode: 'solution', volume: '1000', ratio: '10' });
    expect(draftOfItem(3, { product_id: 7, qty: 40, solution: null })).toEqual({ key: 3, product_id: 7, qty: '40' });
  });
  it('проверка и сборка: раствор или количество', () => {
    const r = validateRecipe('Мойка', [
      { key: 0, product_id: 7, qty: '', mode: 'solution', volume: '1 000', ratio: '10' },
      { key: 1, product_id: 8, qty: '30' },
    ]);
    expect(r).toEqual({ ok: true, value: { name: 'Мойка', items: [{ product_id: 7, solution: sol }, { product_id: 8, qty: 30 }] } });
  });
  it('ошибки раствора по строке', () => {
    const r = validateRecipe('Мойка', [{ key: 0, product_id: 7, qty: '40', mode: 'solution', volume: '', ratio: '2000' }]);
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.errors.byRow[0]?.qty).toBeTruthy();
  });
  it('в режиме раствора количество не проверяется, в обычном — проверяется', () => {
    expect(validateRecipe('М', [{ key: 0, product_id: 7, qty: 'abc', mode: 'solution', volume: '500', ratio: '1' }]).ok).toBe(true);
    expect(validateRecipe('М', [{ key: 0, product_id: 7, qty: 'abc' }]).ok).toBe(false);
  });
  it('применение: раствор сохраняется в строках, смена объёма — изменение', () => {
    const rows = [{ product_id: 7, qty: 91, solution: sol }, { product_id: 8, qty: 0 }];
    expect(applyItems(rows)).toEqual([{ product_id: 7, qty: 91, solution: sol }]);
    expect(qtyChanged([{ product_id: 7, qty: 91, solution: sol }], [{ product_id: 7, qty: 91, solution: sol }])).toBe(false);
    expect(qtyChanged([{ product_id: 7, qty: 91, solution: { volume_ml: 1005, ratio: 10 } }], [{ product_id: 7, qty: 91, solution: sol }])).toBe(true);
    expect(qtyChanged([{ product_id: 7, qty: 91 }], [{ product_id: 7, qty: 91, solution: null }])).toBe(false);
  });
});

describe('форма позиции: пропорции', () => {
  const fields = (f: Partial<ProductFields> = {}): ProductFields => ({
    name: 'Шампунь', brand: '', category_id: 1, unit: 'ml', package_size: '1000', min_stock: '', barcode: '', note: '', purpose_ids: [], ph: '', incompatible: '', instruction: '', ...f,
  });
  it('в запрос идут пропорции в порядке списка, с id существующих', () => {
    const r = validateProduct(fields({ dilutions: draftsOf([{ id: 4, name: 'Салон', ratio: 20, sort: 0 }, { id: 5, name: 'Предмойка', ratio: 10, sort: 1 }]) }));
    expect(r.ok && r.value.dilutions).toEqual([{ id: 4, name: 'Салон', ratio: 20 }, { id: 5, name: 'Предмойка', ratio: 10 }]);
  });
  it('пустой список заменяет прежний; поле не задано — не отправляется', () => {
    const empty = validateProduct(fields({ dilutions: [] }));
    expect(empty.ok && empty.value.dilutions).toEqual([]);
    const none = validateProduct(fields());
    expect(none.ok && 'dilutions' in none.value).toBe(false);
  });
  it('у шт пропорции не отправляются', () => {
    const r = validateProduct(fields({ unit: 'pcs', dilutions: [{ key: 0, name: 'А', ratio: '10' }] }));
    expect(r.ok && 'dilutions' in r.value).toBe(false);
  });
  it('неверная пропорция не даёт сохранить', () => {
    const r = validateProduct(fields({ dilutions: [{ key: 0, name: '', ratio: '2000' }] }));
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.errors.dilutions).toBeTruthy();
  });
});
