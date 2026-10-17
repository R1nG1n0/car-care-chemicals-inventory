// «Добавить в список» (ФТ-7.2): позиция из своих или просто текст («Глина для кузова»).
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { inventory, type ProductSummary } from '../../api';
import { keys } from '../../app/queries';
import { ProductPicker } from '../../components/ProductPicker';
import { Sheet } from '../../components/Sheet';
import { ShopNewProduct } from '../../components/ShopNewProduct';
import { errorText } from '../../components/ShopUi';
import { busyProps, Field } from '../../components/ui';

type Props = { open: boolean; onClose: () => void; excludeIds: number[] };

export function ShopAddSheet({ open, onClose, excludeIds }: Props) {
  const qc = useQueryClient();
  const [mode, setMode] = useState<'product' | 'text'>('product');
  const [title, setTitle] = useState('');
  const [error, setError] = useState('');
  const [creating, setCreating] = useState<string | null>(null);

  const add = useMutation({
    mutationFn: inventory.addShopping,
    onSuccess: async () => {
      setTitle('');
      setError('');
      await qc.invalidateQueries({ queryKey: keys.shopping });
      onClose();
    },
    onError: (err) => setError(errorText(err)),
  });

  const pick = (p: ProductSummary) => !add.isPending && add.mutate({ product_id: p.id });
  const addText = () => {
    const t = title.trim();
    if (t === '') return setError('Напишите, что купить');
    if ([...t].length > 80) return setError('Не длиннее 80 символов');
    if (!add.isPending) add.mutate({ title: t });
  };

  return (
    <>
      <Sheet open={open} onClose={onClose} title="Добавить в список">
        <div className="app-stack">
          <div className="g-seg" role="group" aria-label="Что добавить">
            <button type="button" aria-pressed={mode === 'product'} onClick={() => setMode('product')}>
              Позиция
            </button>
            <button type="button" aria-pressed={mode === 'text'} onClick={() => setMode('text')}>
              Просто текст
            </button>
          </div>
          {mode === 'product' ? (
            <ProductPicker id="shop-add-product" autoFocus excludeIds={excludeIds} onChange={pick} onCreate={setCreating} limit={20} />
          ) : (
            <form
              className="app-stack"
              onSubmit={(e) => {
                e.preventDefault();
                addText();
              }}
            >
              <Field
                id="shop-add-title"
                label="Что купить"
                placeholder="Глина для кузова"
                maxLength={80}
                value={title}
                data-autofocus
                error={error || undefined}
                onChange={(e) => setTitle(e.target.value)}
              />
              <button type="submit" className="g-btn g-btn--block g-press" {...busyProps(add.isPending)}>
                Добавить
              </button>
            </form>
          )}
          {mode === 'product' && error && (
            <p className="g-field__error" role="alert">
              {error}
            </p>
          )}
        </div>
      </Sheet>
      <ShopNewProduct
        name={creating}
        onClose={() => setCreating(null)}
        onCreated={(p) => {
          setCreating(null);
          pick(p);
        }}
      />
    </>
  );
}
