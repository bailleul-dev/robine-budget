import { useEffect, useState } from 'react';
import { ActivityIndicator, Pressable, StyleSheet } from 'react-native';
import { Text } from '@/components/ui/text';
import Svg, { Path } from 'react-native-svg';
import { ApiError } from '@/api/client';
import { useAuth } from '@/auth/AuthContext';
import { useGoogleIdToken } from '@/auth/google';
import { Radius, Spacing } from '@/constants/theme';
import { useTheme } from '@/hooks/use-theme';

function GoogleLogo() {
  return (
    <Svg width={18} height={18} viewBox="0 0 24 24">
      <Path
        fill="#4285F4"
        d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"
      />
      <Path
        fill="#34A853"
        d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"
      />
      <Path
        fill="#FBBC05"
        d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.06H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.94l2.85-2.22.81-.63z"
      />
      <Path
        fill="#EA4335"
        d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.06l3.66 2.84c.87-2.6 3.3-4.52 6.16-4.52z"
      />
    </Svg>
  );
}

/** Only ever mounted when auth/google.ts's googleAuthConfigured is true
 * (see login.tsx) — expo-auth-session's hook throws during render if this
 * platform's client ID is missing, so it can't be called unconditionally
 * and just hidden. */
export function GoogleSignInButton() {
  const theme = useTheme();
  const { loginWithGoogleIdToken } = useAuth();
  const { request, promptAsync, idToken } = useGoogleIdToken();
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!idToken) return;
    let cancelled = false;
    (async () => {
      setError(null);
      setSubmitting(true);
      try {
        await loginWithGoogleIdToken(idToken);
      } catch (err) {
        if (!cancelled) setError(err instanceof ApiError ? err.message : 'Connexion Google impossible.');
      } finally {
        if (!cancelled) setSubmitting(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [idToken, loginWithGoogleIdToken]);

  return (
    <>
      <Pressable
        onPress={() => promptAsync()}
        disabled={!request || submitting}
        style={[styles.button, { borderColor: theme.line, backgroundColor: theme.surface, opacity: !request || submitting ? 0.6 : 1 }]}
      >
        {submitting ? <ActivityIndicator size="small" color={theme.text} /> : <GoogleLogo />}
        <Text style={[styles.buttonText, { color: theme.text }]}>Continuer avec Google</Text>
      </Pressable>
      {error && <Text style={[styles.error, { color: theme.negative }]}>{error}</Text>}
    </>
  );
}

const styles = StyleSheet.create({
  button: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: Spacing.two,
    borderWidth: 1,
    borderRadius: Radius.pill,
    paddingVertical: Spacing.three,
  },
  buttonText: { fontWeight: '700', fontSize: 14 },
  error: { fontSize: 12.5, marginTop: Spacing.one },
});
