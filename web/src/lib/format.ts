// Форматирование чисел для интерфейса: разряды через неразрывный пробел,
// как принято в русском тексте («2 320 мл», «1 190 ₽»).

const qtyFormat = new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 0 });

/** Количество в мл или шт: целое число с разрядами. */
export function formatQty(n: number): string {
  return qtyFormat.format(n);
}

/** Сумма в рублях из копеек (ФТ-4.4): копейки показываются, только если они есть. */
export function formatMoney(kopecks: number): string {
  const rub = kopecks / 100;
  const fmt = new Intl.NumberFormat('ru-RU', {
    minimumFractionDigits: Number.isInteger(rub) ? 0 : 2,
    maximumFractionDigits: 2,
  });
  return `${fmt.format(rub)}\u00a0₽`;
}
