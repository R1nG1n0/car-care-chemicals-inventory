// Количество строки шаблона (ФТ-17.2.4): у позиции в мл с пропорциями — переключатель
// «Концентрат / Раствор»; в режиме раствора вместо числа — объём и пропорция.
// Общий для редактора шаблона и применения.
import type { ProductSummary } from '../../api';
import { ModeSwitch } from '../../components/ModeSwitch';
import { SolutionFields } from '../../components/SolutionFields';
import { dilutionsOf, initialRatio, solutionError, type DilutionMode } from '../../lib/dilution';
import { RecipeQty } from './RecipeQty';

export type LineValue = { qty: string; mode?: DilutionMode | undefined; volume?: string | undefined; ratio?: string | undefined };

type Props = {
  id: string;
  product: Pick<ProductSummary, 'name' | 'unit' | 'dilutions'>;
  value: LineValue;
  /** Подсветить ошибку (текст ошибки показывает экран). */
  invalid?: boolean;
  describedBy?: string | undefined;
  onChange: (patch: Partial<LineValue>) => void;
};

export function RecipeLine({ id, product, value, invalid, describedBy, onChange }: Props) {
  const dilutions = product.unit === 'ml' ? dilutionsOf(product) : [];
  const solution = value.mode === 'solution';
  // Строка, заданная раствором, остаётся такой, даже если пропорции у позиции потом убрали
  const canSwitch = dilutions.length > 0 || solution;
  return (
    <div className="dil-line">
      {canSwitch && (
        <ModeSwitch
          id={`${id}-mode`}
          compact
          mode={solution ? 'solution' : 'concentrate'}
          onChange={(mode) => onChange(mode === 'solution' && !value.ratio ? { mode, ratio: initialRatio(dilutions, null), volume: value.volume ?? '' } : { mode })}
        />
      )}
      {solution ? (
        <SolutionFields
          id={`${id}-sol`}
          compact
          dilutions={dilutions}
          value={{ volume: value.volume ?? '', ratio: value.ratio ?? '' }}
          onChange={(v) => onChange({ volume: v.volume, ratio: v.ratio })}
          errors={invalid ? (solutionError(value.volume ?? '', value.ratio ?? '') ?? undefined) : undefined}
          describedBy={describedBy}
        />
      ) : (
        <RecipeQty id={id} name={product.name} unit={product.unit} value={value.qty} invalid={invalid} describedBy={describedBy} onChange={(qty) => onChange({ qty })} />
      )}
    </div>
  );
}
