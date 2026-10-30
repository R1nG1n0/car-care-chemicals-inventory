// Число, которое при смене значения коротко проявляется (калькулятор, строка
// «Концентрат + вода»). Только transform и opacity; при первом показе не играет.
import { useState } from 'react';
import { formatQty } from '../lib/format';

export function AnimatedNumber({ value }: { value: number }) {
  const text = formatQty(value);
  // Номер смены — ключ: новый элемент проигрывает анимацию ещё раз, а уходящий не держится
  const [state, setState] = useState({ text, n: 0 });
  if (state.text !== text) setState({ text, n: state.n + 1 });
  return (
    <b key={state.n} className={`dil-num${state.n > 0 ? ' is-swap' : ''}`}>
      {text}
    </b>
  );
}
