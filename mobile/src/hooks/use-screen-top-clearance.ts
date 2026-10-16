import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useIsDesktop } from '@/hooks/use-is-desktop';

/** How much top padding a tab's root screen needs for the status bar.
 *
 * On mobile those screens have no header (the selected tab already says
 * where you are — see components/TabStack.tsx), so they run edge to edge
 * and pad their own scrolling content instead. That way a full-bleed
 * background, the Aurora wash in particular, reaches the top of the
 * screen rather than starting below the status bar.
 *
 * 0 on desktop, where the drawer's screens keep a real header above them. */
export function useScreenTopClearance(): number {
  const insets = useSafeAreaInsets();
  const isDesktop = useIsDesktop();
  return isDesktop ? 0 : insets.top;
}
