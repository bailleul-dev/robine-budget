import { useState } from 'react';
import { ActivityIndicator, Pressable, StyleSheet } from 'react-native';
import { Text } from '@/components/ui/text';
import Svg, { Path } from 'react-native-svg';
import { ApiError } from '@/api/client';
import { useAuth } from '@/auth/AuthContext';
import { signInWithRobine } from '@/auth/robine';
import { Radius, Spacing } from '@/constants/theme';
import { useTheme } from '@/hooks/use-theme';

/** Only ever mounted once useAuthConfig() reports robine_enabled (see
 * login.tsx) — unlike GoogleSignInButton there's no client-side config to
 * throw on, but showing this against a backend with Robine unconfigured
 * would just 400 on tap. */
export function RobineSignInButton() {
  const theme = useTheme();
  const { loginWithSessionToken } = useAuth();
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const onPress = async () => {
    setError(null);
    setSubmitting(true);
    try {
      const token = await signInWithRobine();
      if (token) await loginWithSessionToken(token);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Connexion Robine ID impossible.');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <>
      <Pressable
        onPress={onPress}
        disabled={submitting}
        style={[styles.button, { borderColor: theme.line, backgroundColor: theme.surface, opacity: submitting ? 0.6 : 1 }]}
      >
        {submitting ? <ActivityIndicator size="small" color={theme.text} /> : <RobineLogo />}
        <Text style={[styles.buttonText, { color: theme.text }]}>Continuer avec Robine ID</Text>
      </Pressable>
      {error && <Text style={[styles.error, { color: theme.negative }]}>{error}</Text>}
    </>
  );
}

// Same purple mark as the app's own icon/favicon (see mobile/public/favicon.svg's
// outer path, flattened to one solid color) — Robine ID is "our own"
// identity provider, not a third-party brand with a logo to reproduce, so
// reusing the app's own mark reads better than inventing one.
function RobineLogo() {
  return (
    <Svg width={16} height={16} viewBox="0 0 48 46">
      <Path
        fill="#863bff"
        d="M25.946 44.938c-.664.845-2.021.375-2.021-.698V33.937a2.26 2.26 0 0 0-2.262-2.262H10.287c-.92 0-1.456-1.04-.92-1.788l7.48-10.471c1.07-1.497 0-3.578-1.842-3.578H1.237c-.92 0-1.456-1.04-.92-1.788L10.013.474c.214-.297.556-.474.92-.474h28.894c.92 0 1.456 1.04.92 1.788l-7.48 10.471c-1.07 1.498 0 3.579 1.842 3.579h11.377c.943 0 1.473 1.088.89 1.83L25.947 44.94z"
      />
    </Svg>
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
