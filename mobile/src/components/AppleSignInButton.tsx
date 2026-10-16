import { useEffect, useState } from 'react';
import { Platform, StyleSheet } from 'react-native';
import * as AppleAuthentication from 'expo-apple-authentication';
import { Text } from '@/components/ui/text';
import { ApiError } from '@/api/client';
import { useAuth } from '@/auth/AuthContext';
import { Radius, Spacing } from '@/constants/theme';
import { useResolvedScheme } from '@/theme/ThemeProvider';

/** "Sign in with Apple", Apple's own button — App Store guideline 4.8
 * requires it next to any third-party login (Google), and it is the
 * quickest way in on an iPhone anyway. iOS only, and only once the device
 * says it is available (it isn't on an iPad simulator without an Apple
 * account, for instance) and the backend has APPLE_BUNDLE_IDS set (see
 * login.tsx, which reads apple_enabled from GET /api/auth/config).
 *
 * Apple hands over the user's name only on the very first sign-in, and
 * never again, so it goes to the backend with that first call. */
export function AppleSignInButton() {
  const { loginWithAppleIdentityToken } = useAuth();
  const scheme = useResolvedScheme();
  const [available, setAvailable] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (Platform.OS !== 'ios') return;
    AppleAuthentication.isAvailableAsync()
      .then(setAvailable)
      .catch(() => setAvailable(false));
  }, []);

  if (!available) return null;

  const onPress = async () => {
    setError(null);
    try {
      const credential = await AppleAuthentication.signInAsync({
        requestedScopes: [
          AppleAuthentication.AppleAuthenticationScope.FULL_NAME,
          AppleAuthentication.AppleAuthenticationScope.EMAIL,
        ],
      });
      if (!credential.identityToken) throw new Error('no identity token');
      const fullName = [credential.fullName?.givenName, credential.fullName?.familyName].filter(Boolean).join(' ');
      await loginWithAppleIdentityToken(credential.identityToken, fullName);
    } catch (err) {
      // Closing Apple's sheet is a cancel, not a failure worth showing.
      if (err instanceof Error && 'code' in err && err.code === 'ERR_REQUEST_CANCELED') return;
      setError(err instanceof ApiError ? err.message : 'Connexion avec Apple impossible.');
    }
  };

  return (
    <>
      <AppleAuthentication.AppleAuthenticationButton
        buttonType={AppleAuthentication.AppleAuthenticationButtonType.CONTINUE}
        buttonStyle={
          scheme === 'dark'
            ? AppleAuthentication.AppleAuthenticationButtonStyle.WHITE
            : AppleAuthentication.AppleAuthenticationButtonStyle.BLACK
        }
        cornerRadius={Radius.control}
        style={styles.button}
        onPress={onPress}
      />
      {error && <Text style={styles.error}>{error}</Text>}
    </>
  );
}

const styles = StyleSheet.create({
  button: { height: 48, marginBottom: Spacing.two },
  error: { fontSize: 13, textAlign: 'center', marginBottom: Spacing.two, color: '#e0626f' },
});
