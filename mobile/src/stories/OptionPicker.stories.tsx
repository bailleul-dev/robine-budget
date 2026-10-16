import type { Meta, StoryObj } from '@storybook/react-native-web-vite';
import { useState } from 'react';
import { OptionPicker } from '@/components/OptionPicker';

const RECURRENCE_OPTIONS = [
  { value: 'one_off', label: 'Ponctuel' },
  { value: 'perpetual', label: 'Tous les mois' },
  { value: 'fixed_count', label: 'Nombre de mois fixe' },
  { value: 'every_x_weeks', label: 'Toutes les X semaines' },
] as const;

/** The modal-list picker used throughout the app's forms wherever there's
 * no native <select> to fall back on (account, kind, recurrence-type…) —
 * see (app)/planned-items.tsx for a real usage. Wrapped in a small stateful
 * component here since OptionPicker is controlled: Storybook renders the
 * real interaction (tap to open the sheet, pick a row, see it selected),
 * not just a static snapshot. */
function OptionPickerDemo() {
  const [value, setValue] = useState<(typeof RECURRENCE_OPTIONS)[number]['value'] | null>(null);
  return (
    <OptionPicker
      title="Récurrence"
      placeholder="Choisir une récurrence…"
      value={value}
      options={[...RECURRENCE_OPTIONS]}
      onChange={setValue}
    />
  );
}

const meta = {
  title: 'Design System/OptionPicker',
  component: OptionPickerDemo,
} satisfies Meta<typeof OptionPickerDemo>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
