// Количество в строке шаблона (ФТ-8.4 = ФТ-5.8): те же способы, что при расходе
// одной позиции — число вписывается вручную, кнопки −большой −малый +малый +большой
// из профиля (для шт — −1 +1 +5). Общий компонент QtyInput в компактном виде.
import type { Unit } from '../../api';
import { QtyInput } from '../../components/QtyInput';
import { MAX_QTY } from '../../lib/qty';
import { unitLabel } from '../../lib/stock';

type Props = { id: string; name: string; unit: Unit; value: string; invalid?: boolean; describedBy?: string | undefined; onChange: (v: string) => void };

export function RecipeQty({ id, name, unit, value, invalid, describedBy, onChange }: Props) {
  return (
    <QtyInput
      id={id}
      compact
      label={`Количество, ${unitLabel(unit)}: ${name}`}
      value={value}
      onChange={onChange}
      unit={unit}
      min={0}
      max={MAX_QTY}
      invalid={invalid}
      describedBy={describedBy}
    />
  );
}
