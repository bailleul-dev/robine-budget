import { useContext } from 'react';
import { BottomTabBarHeightContext } from 'expo-router/tabs';

/** How much bottom padding a screen's scrollable content needs so its last
 * item isn't hidden behind the floating mobile tab bar (see (app)/_layout.tsx
 * — MobileTabs' tabBarStyle is position:'absolute', so it no longer reserves
 * this space in flow itself). Reads the same height react-navigation
 * computes for the bar and exposes via BottomTabBarHeightContext, so it's
 * always exactly right regardless of device or safe-area insets, with no
 * magic number duplicated here.
 *
 * 0 outside a JS Tabs navigator: desktop's DesktopDrawer (see
 * (app)/_layout.tsx) has no bar to clear, and iOS's native tab bar insets
 * scroll views itself. Reading the context directly (rather than
 * expo-router's useBottomTabBarHeight()) is what makes that 0 instead of a
 * throw. */
export function useTabBarClearance(): number {
  return useContext(BottomTabBarHeightContext) ?? 0;
}
