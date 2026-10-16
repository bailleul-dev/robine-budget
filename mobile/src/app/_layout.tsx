import {
  Manrope_400Regular,
  Manrope_500Medium,
  Manrope_600SemiBold,
  Manrope_700Bold,
  Manrope_800ExtraBold,
} from '@expo-google-fonts/manrope';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useFonts } from 'expo-font';
import { Stack } from 'expo-router';
import Head from 'expo-router/head';
import { ActivityIndicator, Platform, View } from 'react-native';
import { GestureHandlerRootView } from 'react-native-gesture-handler';
import { SafeAreaProvider } from 'react-native-safe-area-context';
import { AuthProvider, useAuth } from '@/auth/AuthContext';
import { useTheme } from '@/hooks/use-theme';
import { ThemeProvider } from '@/theme/ThemeProvider';

const queryClient = new QueryClient();

function RootNavigator() {
  const { user, loading } = useAuth();
  const theme = useTheme();
  // Manrope (see constants/theme.ts Typefaces), held behind the same
  // loading state as auth so text never flashes in a fallback font. A load
  // error falls through to system fonts rather than blocking the app.
  const [fontsLoaded, fontError] = useFonts({
    Manrope_400Regular,
    Manrope_500Medium,
    Manrope_600SemiBold,
    Manrope_700Bold,
    Manrope_800ExtraBold,
  });

  if (loading || (!fontsLoaded && !fontError)) {
    return (
      <View style={{ flex: 1, alignItems: 'center', justifyContent: 'center', backgroundColor: theme.background }}>
        <ActivityIndicator color={theme.accent} />
      </View>
    );
  }

  return (
    <Stack screenOptions={{ headerShown: false }}>
      <Stack.Protected guard={!!user}>
        <Stack.Screen name="(app)" />
      </Stack.Protected>
      <Stack.Protected guard={!user}>
        <Stack.Screen name="login" />
      </Stack.Protected>
      {/* Reachable regardless of auth state, unlike the two groups above —
       * a share link's landing page (see invite/[token].tsx) has to work
       * for a visitor who isn't signed in yet *and* auto-redeem right away
       * for one who already is. */}
      <Stack.Screen name="invite/[token]" />
      {/* Also always reachable, same reason: the Robine ID login popup (see
       * auth/robine.ts) opens this on web before the popup's own /auth/me
       * check has resolved either way — see auth-callback.tsx. */}
      <Stack.Screen name="auth-callback" />
    </Stack>
  );
}

export default function RootLayout() {
  return (
    // Required by expo-router/drawer (pan-to-open/close gestures) — see
    // (app)/_layout.tsx.
    <GestureHandlerRootView style={{ flex: 1 }}>
      {/* Without this, nothing in the tree knows about the notch/Dynamic
       * Island or the home indicator — react-navigation's own chrome
       * (Stack/Drawer headers, and the bottom Tabs bar on mobile, see
       * (app)/_layout.tsx) silently falls back to a naive inset guess
       * instead of the real one from useSafeAreaInsets(). */}
      <SafeAreaProvider>
        {/* expo-router/head manages the web <title> client-side (react-helmet
         * under the hood) and otherwise leaves it empty — the static
         * <title> in +html.tsx alone gets overwritten by an empty one at
         * hydration without this. Web only: on iOS it is Handoff metadata
         * instead, and without an `origin` in the expo-router config plugin
         * it pops an "Expo Head" alert on launch — even in Release builds. */}
        {Platform.OS === 'web' && (
          <Head>
            <title>Lagune</title>
          </Head>
        )}
        {/* Above everything that calls useTheme() (RootNavigator included)
         * so the Settings screen's light/dark/auto switch reaches every
         * screen, not just ones below (app)/_layout.tsx. */}
        <ThemeProvider>
          <QueryClientProvider client={queryClient}>
            <AuthProvider>
              <RootNavigator />
            </AuthProvider>
          </QueryClientProvider>
        </ThemeProvider>
      </SafeAreaProvider>
    </GestureHandlerRootView>
  );
}
