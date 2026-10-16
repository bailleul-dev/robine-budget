import { Stack } from 'expo-router';
import { useIsDesktop } from '@/hooks/use-is-desktop';
import { useTheme } from '@/hooks/use-theme';

/** The Stack inside each top-level tab (see (app)/_layout.tsx). Headers
 * live here rather than on the Drawer/Tabs navigators because iOS's native
 * tab bar has no header of its own, and a Stack per tab also keeps pushed
 * screens (an account's detail) inside their tab, under the tab bar.
 *
 * On mobile the tab's root screen has no header: the selected tab already
 * says where you are. It runs edge to edge, and pads its own content for
 * the status bar (see useScreenTopClearance) so a full-bleed background
 * reaches the top. Pushed screens keep their header, for the title and the
 * back button. */
export function TabStack({ title }: { title: string }) {
  const theme = useTheme();
  const isDesktop = useIsDesktop();

  return (
    <Stack
      screenOptions={{
        headerStyle: { backgroundColor: theme.background },
        headerTintColor: theme.text,
        headerTitleStyle: { fontFamily: 'Manrope_800ExtraBold', fontSize: 19 },
        headerShadowVisible: false,
        contentStyle: { backgroundColor: theme.background },
      }}
    >
      <Stack.Screen
        name="index"
        options={isDesktop ? { title } : { title, headerShown: false }}
      />
    </Stack>
  );
}
