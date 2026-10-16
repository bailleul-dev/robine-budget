/** LAGUNE — money as calm water.
 *
 * Budgets are usually drawn as warnings: red bars, alarm badges, spreadsheets.
 * Lagune goes the other way. The palette comes from a shallow lagoon at
 * dusk: misty blue-gray neutrals, a lagoon teal as the one brand hue, and
 * lilac for everything still ahead of you (planned items, forecasts), so
 * the future reads as dreamy and open rather than as a debt. Spending is a
 * soft coral, never alarm red.
 *
 * Shape language is smooth and organic: large radii (cards 24, controls 16,
 * chips fully round), blurred "aurora" gradients behind key screens (see
 * components/ui/aurora.tsx) and frosted-glass cards floating on them. Type
 * is Manrope: geometric but warm, with good tabular figures for amounts.
 *
 * Token names are shared with the web app's App.css custom properties so
 * the two clients can keep the same vocabulary. */
import { Platform } from 'react-native';

export const Colors = {
  light: {
    background: '#f3f6f8',
    surface: '#ffffff',
    surfaceSoft: '#eaf2f3',
    glass: 'rgba(255, 255, 255, 0.62)',
    glassBorder: 'rgba(255, 255, 255, 0.9)',
    text: '#1c2b36',
    textSecondary: '#5e7080',
    subtle: '#9aa9b4',
    line: '#e2e9ed',
    accent: '#23a898',
    accentDark: '#13806f',
    accentPale: '#d6f3ef',
    onAccent: '#ffffff',
    lilac: '#a99bf2',
    brandYellow: '#f7c969',
    negative: '#e0626f',
    negativeBg: '#fde9ea',
    positive: '#1f9e78',
    positiveBg: '#dcf4eb',
    forecastBg: '#eeeafe',
    forecastText: '#6a5ad3',
  },
  dark: {
    background: '#0d1820',
    surface: '#14232d',
    surfaceSoft: '#1b2e39',
    glass: 'rgba(22, 38, 48, 0.6)',
    glassBorder: 'rgba(255, 255, 255, 0.08)',
    text: '#e7f0f3',
    textSecondary: '#9bafba',
    subtle: '#647884',
    line: '#233743',
    accent: '#3ccfbc',
    accentDark: '#72e2d3',
    accentPale: '#123a39',
    onAccent: '#06201d',
    lilac: '#b8adff',
    brandYellow: '#f7c969',
    negative: '#ff8e97',
    negativeBg: '#3a1f28',
    positive: '#52d6a8',
    positiveBg: '#113529',
    forecastBg: '#25224a',
    forecastText: '#c3b9ff',
  },
} as const;

export type ThemeColor = keyof typeof Colors.light & keyof typeof Colors.dark;

/** Manrope, one registered family per weight (that's how custom fonts work
 * on native — see useFonts() in app/_layout.tsx). Screens keep writing
 * plain `fontWeight`; the app's <Text> (components/ui/text.tsx) resolves it
 * to the right family. */
export const Typefaces = {
  400: 'Manrope_400Regular',
  500: 'Manrope_500Medium',
  600: 'Manrope_600SemiBold',
  700: 'Manrope_700Bold',
  800: 'Manrope_800ExtraBold',
} as const;

export const Fonts = Platform.select({
  ios: { sans: Typefaces[400], mono: 'ui-monospace' },
  default: { sans: Typefaces[400], mono: 'monospace' },
  web: { sans: Typefaces[400], mono: 'ui-monospace, monospace' },
});

/** Smooth, pebble-like corners. `pill` is for chips and tags. */
export const Radius = {
  control: 16,
  card: 24,
  sheet: 28,
  pill: 999,
} as const;

/** Soft, tinted elevation instead of hairline borders: cards float on the
 * mist background like pebbles under shallow water. `boxShadow` is
 * cross-platform on the new architecture. */
export const Elevation = {
  card: { boxShadow: '0px 6px 20px rgba(28, 75, 90, 0.07)' },
  raised: { boxShadow: '0px 12px 32px rgba(28, 75, 90, 0.14)' },
  button: { boxShadow: '0px 6px 16px rgba(35, 168, 152, 0.3)' },
} as const;

export const Spacing = {
  half: 2,
  one: 4,
  two: 8,
  three: 16,
  four: 24,
  five: 32,
  six: 64,
} as const;

export const MaxContentWidth = 800;

/** Below this window width: mobile-style single column + slide-out drawer.
 * At or above it: permanent sidebar + side-by-side content columns. Checked
 * via useWindowDimensions() (not a media query — this is React Native, not
 * CSS) wherever a screen's layout needs to branch on it. */
export const DesktopBreakpoint = 900;

/** Sidebar width in permanent (desktop) drawer mode. */
export const DrawerWidth = 260;

/** Sidebar width when collapsed to icons-only — desktop only, toggled from
 * the drawer's own header button (see (app)/_layout.tsx). Wide enough for
 * DrawerItem's fixed internal padding (16 start + 24 icon + 24 end) without
 * clipping. */
export const RailWidth = 80;
