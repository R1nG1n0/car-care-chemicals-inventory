// Строка новой покупки (макет «Новая покупка»): позиция, упаковок ±, цена за упаковку, место.
import { IconMinus, IconPlus, IconX } from '@tabler/icons-react';
import type { Category, Location, ProductSummary } from '../../api';
import { ProductPicker } from '../../components/ProductPicker';
import { ShopRefSelect } from '../../components/ShopRefSelect';
import { catClass } from '../../components/ShopUi';
import { Ico } from '../../components/ui';
import { formatMoney } from '../../lib/format';
import { clampPackages, lineTotal, MAX_PACKAGES, type LineDraft, type LineErrors } from '../../lib/shopPurchase';
import { unitLabel } from '../../lib/stock';

type Props = {
  n: number;
  line: LineDraft;
  product: ProductSummary | undefined;
  category: Category | undefined;
  locations: Location[];
  excludeIds: number[];
  errors: LineErrors | undefined;
  canRemove: boolean;
  onChange: (l: LineDraft) => void;
  onRemove: () => void;
  onCreateProduct: (name: string) => void;
  onCreateLocation: (name: string) => Promise<number>;
};

export function PurchaseLineCard({ n, line, product, category, locations, excludeIds, errors, canRemove, onChange, onRemove, onCreateProduct, onCreateLocation }: Props) {
  const id = `pl-${line.key}`;
  const set = (p: Partial<LineDraft>) => onChange({ ...line, ...p });
  const pkg = (d: number) => set({ packages: clampPackages(line.packages + d) });
  const sum = lineTotal(line);

  return (
    <section className={`g-card shop-line ${catClass(category?.color)}`} aria-labelledby={`${id}-h`}>
      <div className="g-card__head">
        <span id={`${id}-h`}>
          Строка {n}
          {category ? ` · ${category.name}` : ''}
        </span>
        {canRemove ? (
          <button type="button" className="g-btn g-btn--icon g-btn--ghost" aria-label={`Удалить строку ${n}`} onClick={onRemove}>
            <Ico icon={IconX} />
          </button>
        ) : (
          <span className="shop-head-pad" />
        )}
      </div>
      <div className="g-card__body">
        <div className={errors?.product ? 'g-field--error' : undefined}>
          <ProductPicker
            id={`${id}-product`}
            value={line.product_id}
            excludeIds={excludeIds}
            onChange={(p) => set({ product_id: p.id })}
            onCreate={onCreateProduct}
            limit={8}
          />
          {errors?.product && (
            <span className="g-field__error" role="alert">
              {errors.product}
            </span>
          )}
        </div>
        <div className="shop-two">
          <div className={`g-field${errors?.packages ? ' g-field--error' : ''}`}>
            <span className="g-label" id={`${id}-pkg`}>
              Упаковок{product ? ` по ${product.package_size} ${unitLabel(product.unit)}` : ''}
            </span>
            <div className="shop-pkg" role="group" aria-labelledby={`${id}-pkg`}>
              <button type="button" className="g-btn g-btn--icon g-press" aria-label="Меньше упаковок" onClick={() => pkg(-1)} aria-disabled={line.packages <= 1 || undefined}>
                <Ico icon={IconMinus} />
              </button>
              <span className="g-num" aria-live="polite">
                {line.packages}
              </span>
              <button type="button" className="g-btn g-btn--icon g-press" aria-label="Больше упаковок" onClick={() => pkg(1)} aria-disabled={line.packages >= MAX_PACKAGES || undefined}>
                <Ico icon={IconPlus} />
              </button>
            </div>
            {errors?.packages && <span className="g-field__error">{errors.packages}</span>}
          </div>
          <div className={`g-field${errors?.price ? ' g-field--error' : ''}`}>
            <label className="g-label" htmlFor={`${id}-price`}>
              Цена за упаковку, ₽
            </label>
            <input
              id={`${id}-price`}
              className="g-input"
              inputMode="decimal"
              autoComplete="off"
              placeholder="0"
              value={line.price}
              aria-invalid={errors?.price ? true : undefined}
              aria-describedby={errors?.price ? `${id}-price-err` : `${id}-price-hint`}
              onChange={(e) => set({ price: e.target.value })}
            />
            {errors?.price ? (
              <span className="g-field__error" id={`${id}-price-err`}>
                {errors.price}
              </span>
            ) : (
              <span className="g-field__hint" id={`${id}-price-hint`}>
                {line.packages > 1 && sum > 0 ? `за ${line.packages}: ${formatMoney(sum)}` : 'с копейками через запятую'}
              </span>
            )}
          </div>
        </div>
        <ShopRefSelect
          id={`${id}-loc`}
          label="Место"
          value={line.location_id}
          onChange={(location_id) => set({ location_id })}
          options={locations}
          noneLabel="Как в прошлый раз"
          newLabel="Новое место…"
          newPlaceholder="Полка 2"
          onCreate={onCreateLocation}
        />
      </div>
    </section>
  );
}
