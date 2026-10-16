import { useWindowDimensions } from 'react-native';
import { DesktopBreakpoint } from '@/constants/theme';

/** True at/above DesktopBreakpoint — used to switch between the mobile
 * single-column + slide-out-drawer layout and the desktop permanent-
 * sidebar + side-by-side-columns one. React Native has no media queries,
 * so this is the one place every screen's responsive branching reads from. */
export function useIsDesktop(): boolean {
  const { width } = useWindowDimensions();
  return width >= DesktopBreakpoint;
}
