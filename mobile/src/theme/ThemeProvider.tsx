import AsyncStorage from '@react-native-async-storage/async-storage';
import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
  type PropsWithChildren,
} from 'react';
import { Colors } from '@/constants/theme';
import { useColorScheme as useSystemColorScheme } from '@/hooks/use-color-scheme';

export type ThemePreference = 'light' | 'dark' | 'system';
type ResolvedScheme = 'light' | 'dark';

const STORAGE_KEY = 'budget_theme_preference';

type ThemeContextValue = {
  /** What the user picked in Settings — 'system' follows the OS. */
  preference: ThemePreference;
  setPreference: (preference: ThemePreference) => void;
  /** 'system' resolved down to an actual light/dark value to render with. */
  scheme: ResolvedScheme;
  colors: (typeof Colors)[ResolvedScheme];
};

const ThemeContext = createContext<ThemeContextValue | null>(null);

function isThemePreference(value: string | null): value is ThemePreference {
  return value === 'light' || value === 'dark' || value === 'system';
}

/** Wraps the whole app (see root _layout.tsx) so every screen's useTheme()
 * call reflects the user's Settings choice instead of only ever following
 * the OS. Defaults to 'system' (today's behavior) until AsyncStorage's read
 * resolves, so most users never see the preference change anything. */
export function ThemeProvider({ children }: PropsWithChildren) {
  const systemScheme = useSystemColorScheme();
  const [preference, setPreferenceState] = useState<ThemePreference>('system');

  useEffect(() => {
    let cancelled = false;
    AsyncStorage.getItem(STORAGE_KEY)
      .then((stored) => {
        if (!cancelled && isThemePreference(stored)) {
          setPreferenceState(stored);
        }
      })
      .catch(() => {
        // No persisted preference (or storage unavailable) — 'system' stays.
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const setPreference = (next: ThemePreference) => {
    setPreferenceState(next);
    AsyncStorage.setItem(STORAGE_KEY, next).catch(() => {
      // Best-effort persistence — the in-memory preference above still
      // applies for the rest of this session even if the write fails.
    });
  };

  const scheme: ResolvedScheme =
    preference === 'system' ? (systemScheme === 'dark' ? 'dark' : 'light') : preference;

  const value = useMemo<ThemeContextValue>(
    () => ({ preference, setPreference, scheme, colors: Colors[scheme] }),
    [preference, scheme],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

function useThemeContext(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) {
    throw new Error('useThemeContext must be used within a ThemeProvider');
  }
  return ctx;
}

/** The resolved color tokens for the current theme — same shape and call
 * site every screen already uses via @/hooks/use-theme. */
export function useThemeColors() {
  return useThemeContext().colors;
}

/** The user's raw preference plus a setter — for the Settings switch. */
export function useThemePreference() {
  const { preference, setPreference } = useThemeContext();
  return { preference, setPreference };
}

/** The resolved 'light'/'dark' scheme itself (as opposed to the color
 * tokens) — for the rare spot that needs to pick between two platform
 * enums rather than a color, e.g. BlurView's tint on the floating tab bar. */
export function useResolvedScheme() {
  return useThemeContext().scheme;
}
