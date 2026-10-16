// Заглушки разделов до этапа 3: заголовок и пустое состояние в стиле системы.
import type { TablerIcon } from '@tabler/icons-react';
import { Empty } from '../components/ui';

type Props = { id: string; title: string; icon: TablerIcon; empty: string; text: string };

export function StubScreen({ id, title, icon, empty, text }: Props) {
  return (
    <div className="app-stack app-stack--fill">
      <h1 className="g-title">{title}</h1>
      <Empty id={`${id}-empty`} icon={icon} title={empty} text={text} />
    </div>
  );
}
