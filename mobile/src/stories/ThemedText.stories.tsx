import type { Meta, StoryObj } from '@storybook/react-native-web-vite';
import { ThemedText } from '@/components/themed-text';

const meta = {
  title: 'Design System/ThemedText',
  component: ThemedText,
  argTypes: {
    type: {
      control: 'select',
      options: ['default', 'title', 'subtitle', 'small', 'smallBold', 'link', 'linkPrimary', 'code'],
    },
    themeColor: {
      control: 'select',
      options: ['text', 'textSecondary', 'subtle', 'accent', 'accentDark', 'positive', 'negative'],
    },
  },
  args: {
    children: 'Votre budget grandit avec vous.',
  },
} satisfies Meta<typeof ThemedText>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Play with the `type` and `themeColor` controls below to see every
 * typographic style + theme color combination this component supports. */
export const Playground: Story = {
  args: { type: 'default' },
};

export const Title: Story = {
  args: { type: 'title', children: 'Lagune' },
};

export const Subtitle: Story = {
  args: { type: 'subtitle', children: 'Vos comptes' },
};

export const Positive: Story = {
  name: 'Amount — Positive',
  args: { type: 'smallBold', themeColor: 'positive', children: '+23,09 €' },
};

export const Negative: Story = {
  name: 'Amount — Negative',
  args: { type: 'smallBold', themeColor: 'negative', children: '-39,40 €' },
};
