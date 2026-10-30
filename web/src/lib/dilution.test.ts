import { describe, expect, it } from 'vitest';
import { concentrateFor, concentrateOf, dilutionsText, draftsOf, fromConcentrate, initialRatio, loadPref, mixText, movementSolution, parseSolution, quickVolumeText, savePref, solutionError, solutionJournalText, solutionQueueText, validateDilutions, waterFor, type DilutionDraft } from './dilution';
import { memoryStorage } from './testStorage';

const norm = (s: string) => s.replace(/\s/gu, ' ');

describe('концентрат', () => {
  it('объём / (N + 1), к ближайшему, половины вверх', () => {
    expect(concentrateFor(1000, 10)).toBe(91);
    expect(concentrateFor(500, 1)).toBe(250);
    expect(concentrateFor(1, 1000)).toBe(1);
    expect(concentrateFor(3, 1)).toBe(2); // 1,5 -> 2
    expect(concentrateFor(5, 3)).toBe(1); // 1,25 -> 1
    expect(concentrateFor(7, 3)).toBe(2); // 1,75 -> 2
    expect(concentrateFor(750, 20)).toBe(36); // 35,71
    expect(concentrateFor(100_000, 1000)).toBe(100);
  });
  it('не меньше 1 мл', () => {
    expect(concentrateFor(10, 1000)).toBe(1);
    expect(concentrateOf({ volume_ml: 100, ratio: 999 })).toBe(1);
  });
  it('вода — остаток объёма', () => {
    expect(waterFor(1000, 10)).toBe(909);
    expect(waterFor(1, 1000)).toBe(0);
  });
  it('обратный расчёт: раствор и вода из концентрата', () => {
    expect(fromConcentrate(100, 10)).toEqual({ solution: 1100, water: 1000 });
    expect(fromConcentrate(1, 1)).toEqual({ solution: 2, water: 1 });
  });
});

describe('раствор в полях ввода', () => {
  it('разбирает и проверяет границы', () => {
    expect(parseSolution('1 000', '10')).toEqual({ volume_ml: 1000, ratio: 10 });
    expect(parseSolution('0', '10')).toBeNull();
    expect(parseSolution('100001', '10')).toBeNull();
    expect(parseSolution('100000', '1000')).toEqual({ volume_ml: 100000, ratio: 1000 });
    expect(parseSolution('500', '0')).toBeNull();
    expect(parseSolution('500', '1001')).toBeNull();
    expect(parseSolution('', '')).toBeNull();
  });
  it('называет, какое поле не так', () => {
    expect(solutionError('500', '10')).toBeNull();
    expect(solutionError('', '10')?.volume).toBeTruthy();
    expect(solutionError('500', '')?.ratio).toBeTruthy();
    expect(solutionError('', '')).toMatchObject({ volume: expect.any(String), ratio: expect.any(String) });
  });
});

describe('подписи', () => {
  it('смесь, очередь, журнал', () => {
    expect(norm(mixText({ volume_ml: 1000, ratio: 10 }))).toBe('Концентрат 91 мл + вода 909 мл');
    expect(norm(solutionQueueText({ volume_ml: 1000, ratio: 10 }))).toBe('Раствор 1 000 мл 1:10 (91 мл)');
    expect(norm(solutionJournalText({ volume_ml: 1000, ratio: 10 }, 91))).toBe('Раствор 1 000 мл 1:10 → 91 мл');
  });
  it('быстрые объёмы', () => {
    expect([500, 750, 1000, 5000].map((v) => norm(quickVolumeText(v)))).toEqual(['500 мл', '750 мл', '1 л', '5 л']);
  });
  it('пропорции позиции одной строкой', () => {
    expect(dilutionsText([{ name: 'Предмойка', ratio: 10 }, { name: 'Салон', ratio: 20 }])).toBe('Предмойка 1:10 · Салон 1:20');
  });
  it('раствор движения журнала', () => {
    expect(movementSolution({ solution_ml: 1000, solution_ratio: 10 })).toEqual({ volume_ml: 1000, ratio: 10 });
    expect(movementSolution({ solution_ml: null, solution_ratio: null })).toBeNull();
    expect(movementSolution({})).toBeNull();
  });
});

const draft = (key: number, name: string, ratio: string, id?: number): DilutionDraft => ({ key, name, ratio, ...(id !== undefined ? { id } : {}) });

describe('пропорции позиции: проверка', () => {
  it('пустой список — норма', () => {
    expect(validateDilutions([])).toEqual({ ok: true, value: [] });
  });
  it('обрезает пробелы, сохраняет id и порядок', () => {
    const r = validateDilutions([draft(0, ' Предмойка ', '10', 5), draft(1, 'Салон', '20')]);
    expect(r).toEqual({ ok: true, value: [{ id: 5, name: 'Предмойка', ratio: 10 }, { name: 'Салон', ratio: 20 }] });
  });
  it('название 1–40 символов', () => {
    const r = validateDilutions([draft(0, '  ', '10'), draft(1, 'я'.repeat(41), '10'), draft(2, 'я'.repeat(40), '10')]);
    expect(r.ok).toBe(false);
    if (!r.ok) {
      expect(r.rows[0]?.name).toBeTruthy();
      expect(r.rows[1]?.name).toBeTruthy();
      expect(r.rows[2]).toBeUndefined();
    }
  });
  it('N от 1 до 1000, целое', () => {
    for (const bad of ['', '0', '1001', '1,5', 'abc']) {
      const r = validateDilutions([draft(0, 'А', bad)]);
      expect(r.ok, bad).toBe(false);
    }
    expect(validateDilutions([draft(0, 'А', '1'), draft(1, 'Б', '1000')]).ok).toBe(true);
  });
  it('не больше 10', () => {
    const many = Array.from({ length: 11 }, (_, i) => draft(i, `П${i}`, '10'));
    const r = validateDilutions(many);
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.form).toBeTruthy();
    expect(validateDilutions(many.slice(0, 10)).ok).toBe(true);
  });
  it('строки с сервера → черновики', () => {
    expect(draftsOf([{ id: 3, name: 'А', ratio: 10, sort: 0 }])).toEqual([{ key: 0, id: 3, name: 'А', ratio: '10' }]);
  });
});

describe('запоминание режима', () => {
  it('сохраняет и читает по позиции', () => {
    const st = memoryStorage();
    expect(loadPref(7, st)).toBeNull();
    savePref(7, { mode: 'solution', ratio: 20 }, st);
    expect(loadPref(7, st)).toEqual({ mode: 'solution', ratio: 20 });
    expect(loadPref(8, st)).toBeNull();
  });
  it('повреждённое и закрытое хранилище не ломает', () => {
    const st = memoryStorage();
    st.setItem('garage:dilution:7', '{не json');
    expect(loadPref(7, st)).toBeNull();
    st.setItem('garage:dilution:7', JSON.stringify({ mode: 'x' }));
    expect(loadPref(7, st)).toBeNull();
    st.setItem('garage:dilution:7', JSON.stringify({ mode: 'concentrate', ratio: 5000 }));
    expect(loadPref(7, st)).toEqual({ mode: 'concentrate', ratio: null });
    const broken = { getItem: () => { throw new Error('закрыто'); }, setItem: () => { throw new Error('закрыто'); } };
    expect(loadPref(7, broken)).toBeNull();
    expect(() => savePref(7, { mode: 'solution', ratio: 10 }, broken)).not.toThrow();
    expect(loadPref(7, null)).toBeNull();
  });
  it('начальная пропорция: запомненная, иначе первая, иначе пусто', () => {
    const list = [{ id: 1, name: 'А', ratio: 10, sort: 0 }, { id: 2, name: 'Б', ratio: 20, sort: 1 }];
    expect(initialRatio(list, { mode: 'solution', ratio: 20 })).toBe('20');
    expect(initialRatio(list, { mode: 'solution', ratio: 7 })).toBe('7');
    expect(initialRatio(list, null)).toBe('10');
    expect(initialRatio([], null)).toBe('');
  });
});
