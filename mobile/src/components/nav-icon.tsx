import type { AndroidSymbol, SFSymbol } from 'expo-symbols';
import { SymbolView } from 'expo-symbols';

export type IconName = { ios: SFSymbol; android: AndroidSymbol; web: AndroidSymbol };

/** SF Symbol on iOS, Material Symbol on Android/web (expo-symbols'
 * cross-platform name object — see SymbolModule.types.ts). Kept in one
 * place so the drawer's nav items, its collapse toggle, and the settings
 * screen's sign-out action all draw from the same small icon set. */
export const NavIcons = {
  overview: { ios: 'house.fill', android: 'home', web: 'home' },
  accounts: { ios: 'building.columns.fill', android: 'account_balance', web: 'account_balance' },
  graphs: { ios: 'chart.line.uptrend.xyaxis', android: 'show_chart', web: 'show_chart' },
  plannedItems: { ios: 'repeat', android: 'repeat', web: 'repeat' },
  categories: { ios: 'tag.fill', android: 'sell', web: 'sell' },
  settings: { ios: 'gearshape.fill', android: 'settings', web: 'settings' },
  logout: { ios: 'rectangle.portrait.and.arrow.right.fill', android: 'logout', web: 'logout' },
  collapseDrawer: { ios: 'chevron.left', android: 'chevron_left', web: 'chevron_left' },
  expandDrawer: { ios: 'chevron.right', android: 'chevron_right', web: 'chevron_right' },
  comment: { ios: 'bubble.left', android: 'chat_bubble', web: 'chat_bubble' },
  themeLight: { ios: 'sun.max.fill', android: 'light_mode', web: 'light_mode' },
  themeDark: { ios: 'moon.fill', android: 'dark_mode', web: 'dark_mode' },
  themeAuto: { ios: 'circle.lefthalf.filled', android: 'brightness_auto', web: 'brightness_auto' },
  delete: { ios: 'trash', android: 'delete', web: 'delete' },
  previous: { ios: 'chevron.left', android: 'chevron_left', web: 'chevron_left' },
  next: { ios: 'chevron.right', android: 'chevron_right', web: 'chevron_right' },
} as const satisfies Record<string, IconName>;

export function NavIcon({ name, color, size }: { name: IconName; color: string; size: number }) {
  return <SymbolView name={name} tintColor={color} size={size} style={{ width: size, height: size }} />;
}
