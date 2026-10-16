import type { Preview } from '@storybook/react-native-web-vite';
import type { ReactNode } from 'react';
import { View } from 'react-native';
import { useTheme } from '../src/hooks/use-theme';
import { ThemeProvider } from '../src/theme/ThemeProvider';

// Every real screen sits inside ThemeProvider (see app/_layout.tsx) and
// picks up its background from useTheme() — without this wrapper here,
// every story either crashes (useTheme() has no context to read) or
// renders on Storybook's own stark white canvas, neither of which is what
// the component actually looks like in the app.
function StoryFrame({ children }: { children: ReactNode }) {
  const theme = useTheme();
  return <View style={{ minHeight: '100vh', backgroundColor: theme.background, padding: 24 }}>{children}</View>;
}

const preview: Preview = {
  parameters: {
    controls: {
      matchers: {
        color: /(background|color)$/i,
        date: /Date$/i,
      },
    },

    a11y: {
      // 'todo' - show a11y violations in the test UI only
      // 'error' - fail CI on a11y violations
      // 'off' - skip a11y checks entirely
      test: 'todo',
    },
    // StoryFrame below already paints the real themed background — the
    // addon's own backgrounds picker would just fight it.
    backgrounds: { disable: true },
  },
  decorators: [
    (Story) => (
      <ThemeProvider>
        <StoryFrame>
          <Story />
        </StoryFrame>
      </ThemeProvider>
    ),
  ],
};

export default preview;
