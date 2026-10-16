import { useEffect } from 'react';
import * as WebBrowser from 'expo-web-browser';
import { ActivityIndicator, StyleSheet, View } from 'react-native';
import { Text } from '@/components/ui/text';
import { Spacing } from '@/constants/theme';
import { useTheme } from '@/hooks/use-theme';

/** Web-only landing page for the Robine ID OIDC round trip (see
 * auth/robine.ts + backend's RobineCallback) — opened as a popup, never
 * navigated to directly. maybeCompleteAuthSession() hands this page's URL
 * (carrying the session token in its query string) back to the tab that
 * opened the popup via postMessage, then closes it; that tab's own
 * signInWithRobine() call is what actually reads the token and signs in,
 * not this screen. Reachable regardless of auth state, like invite/[token]
 * — see the root layout's always-visible Stack.Screen for this route. On
 * native this route is never opened at all: the OS intercepts the
 * dev.bailleul.budget:// callback before it's ever "loaded" as a page. */
export default function AuthCallbackScreen() {
  const theme = useTheme();

  useEffect(() => {
    WebBrowser.maybeCompleteAuthSession();
  }, []);

  return (
    <View style={[styles.screen, { backgroundColor: theme.background }]}>
      <ActivityIndicator color={theme.accent} />
      <Text style={[styles.text, { color: theme.textSecondary }]}>Connexion en cours…</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: Spacing.two },
  text: { fontSize: 13 },
});
