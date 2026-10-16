import { useState } from 'react';
import { KeyboardAvoidingView, Platform, Pressable, StyleSheet, View } from 'react-native';
import { Text, TextInput } from '@/components/ui/text';
import { api, ApiError } from '@/api/client';
import { useAuthConfig } from '@/api/hooks/useAuthConfig';
import { useAuth } from '@/auth/AuthContext';
import { googleAuthConfigured } from '@/auth/google';
import { AppleSignInButton } from '@/components/AppleSignInButton';
import { GoogleSignInButton } from '@/components/GoogleSignInButton';
import { RobineSignInButton } from '@/components/RobineSignInButton';
import { Aurora } from '@/components/ui/aurora';
import { LogoMark } from '@/components/ui/logo';
import { Elevation, Radius, Spacing } from '@/constants/theme';
import { useTheme } from '@/hooks/use-theme';

// The dev user seeded by the Go backend (SEED_DEV_USER in backend/.env),
// dev builds only. __DEV__ is Metro's dev-vs-release flag (the RN
// equivalent of Vite's import.meta.env.DEV) and is stripped from
// production/release builds.
//
// This used to auto-fill the form on load (mirroring
// frontend/src/pages/LoginPage.tsx), but that silently signs you into the
// seed account on every reload with no visible cue — its accounts happen to
// share names with a real synced account, which twice led to real confusion
// debugging features (nothing to sync, no recurring suggestions) against
// the wrong, mostly-empty account. Now it's an explicit tap instead.
// Mirrors the server's password policy (services.MinPasswordLength) so the
// user hears about it before a round trip; the server stays the authority.
const MIN_PASSWORD_LENGTH = 10;

const devSeedEmail = 'dev@example.com';
const devSeedPassword = 'devpass123';

/** Google sign-in (see auth/google.ts + GoogleSignInButton) runs
 * client-side via expo-auth-session — unlike the web app's server-redirect
 * flow (GET /api/auth/google/login), which doesn't translate to a native
 * app that has no browser to redirect through. Only rendered when this
 * platform actually has a client ID configured (googleAuthConfigured) —
 * the button's own hook throws during render otherwise.
 *
 * Robine ID sign-in (see auth/robine.ts + RobineSignInButton) still goes
 * through a server redirect (Robine has no native SDK/ID-token shortcut
 * like Google's), just via an in-app browser instead of a full page
 * navigation — see expo-web-browser's openAuthSessionAsync. Whether to show
 * it comes from the backend (useAuthConfig, GET /api/auth/config) rather
 * than a client-side env var, since there's no per-platform client ID to
 * bake into the app for it. */
export default function LoginScreen() {
  const { login, register } = useAuth();
  const authConfigQuery = useAuthConfig();
  const robineAuthConfigured = authConfigQuery.data?.robine_enabled ?? false;
  // The button hides itself when the device can't do it (see
  // AppleSignInButton); this only says the backend can verify the token.
  const appleAuthConfigured = (authConfigQuery.data?.apple_enabled ?? false) && Platform.OS === 'ios';
  const [mode, setMode] = useState<'login' | 'register' | 'forgot'>('login');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const theme = useTheme();

  const switchMode = (next: 'login' | 'register' | 'forgot') => {
    setMode(next);
    setError(null);
    setNotice(null);
  };

  const onSubmit = async () => {
    setError(null);
    setNotice(null);
    if (mode === 'register' && password.length < MIN_PASSWORD_LENGTH) {
      setError(`Le mot de passe doit contenir au moins ${MIN_PASSWORD_LENGTH} caractères.`);
      return;
    }
    setSubmitting(true);
    try {
      if (mode === 'forgot') {
        // The server answers the same way whether or not the address has an
        // account, so neither can this screen.
        await api.post('/auth/forgot-password', { email });
        setNotice('Si un compte existe pour cette adresse, un e-mail vient de lui être envoyé.');
      } else if (mode === 'login') {
        await login(email, password);
      } else {
        await register(email, password, displayName);
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Une erreur est survenue');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <KeyboardAvoidingView
      behavior={Platform.OS === 'ios' ? 'padding' : undefined}
      style={[styles.screen, { backgroundColor: theme.background }]}
    >
      <Aurora />
      <View style={styles.brand}>
        <LogoMark size={64} />
        <Text style={[styles.title, { color: theme.text }]}>lagune</Text>
        <Text style={[styles.tagline, { color: theme.textSecondary }]}>Votre argent, tout en douceur.</Text>
      </View>
      <View style={[styles.card, { backgroundColor: theme.glass, borderColor: theme.glassBorder }]}>

        {(googleAuthConfigured || robineAuthConfigured || appleAuthConfigured) && (
          <>
            {appleAuthConfigured && <AppleSignInButton />}
            {googleAuthConfigured && <GoogleSignInButton />}
            {robineAuthConfigured && <RobineSignInButton />}
            <View style={styles.divider}>
              <View style={[styles.dividerLine, { backgroundColor: theme.line }]} />
              <Text style={[styles.dividerText, { color: theme.subtle }]}>ou</Text>
              <View style={[styles.dividerLine, { backgroundColor: theme.line }]} />
            </View>
          </>
        )}

        <View style={[styles.tabs, { backgroundColor: theme.surfaceSoft }]}>
          <Pressable
            style={[styles.tab, mode !== 'register' && [styles.tabActive, { backgroundColor: theme.surface }]]}
            onPress={() => switchMode('login')}
          >
            <Text style={[styles.tabText, { color: mode !== 'register' ? theme.accentDark : theme.textSecondary }]}>
              Se connecter
            </Text>
          </Pressable>
          <Pressable
            style={[styles.tab, mode === 'register' && [styles.tabActive, { backgroundColor: theme.surface }]]}
            onPress={() => switchMode('register')}
          >
            <Text style={[styles.tabText, { color: mode === 'register' ? theme.accentDark : theme.textSecondary }]}>
              Créer un compte
            </Text>
          </Pressable>
        </View>

        {mode === 'register' && (
          <View style={styles.field}>
            <Text style={[styles.label, { color: theme.textSecondary }]}>Nom</Text>
            <TextInput
              value={displayName}
              onChangeText={setDisplayName}
              style={[styles.input, { borderColor: theme.line, backgroundColor: theme.surface, color: theme.text }]}
              placeholderTextColor={theme.subtle}
            />
          </View>
        )}
        <View style={styles.field}>
          <Text style={[styles.label, { color: theme.textSecondary }]}>E-mail</Text>
          <TextInput
            testID="login-email"
            value={email}
            onChangeText={setEmail}
            autoCapitalize="none"
            keyboardType="email-address"
            autoComplete="email"
            style={[styles.input, { borderColor: theme.line, backgroundColor: theme.surface, color: theme.text }]}
            placeholderTextColor={theme.subtle}
          />
        </View>
        {mode !== 'forgot' && (
          <View style={styles.field}>
            <Text style={[styles.label, { color: theme.textSecondary }]}>Mot de passe</Text>
            <TextInput
              testID="login-password"
              value={password}
              onChangeText={setPassword}
              secureTextEntry
              autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
              style={[styles.input, { borderColor: theme.line, backgroundColor: theme.surface, color: theme.text }]}
              placeholderTextColor={theme.subtle}
            />
            {mode === 'register' && (
              <Text style={[styles.hint, { color: theme.subtle }]}>
                {MIN_PASSWORD_LENGTH} caractères minimum.
              </Text>
            )}
          </View>
        )}

        {error && <Text style={[styles.error, { color: theme.negative }]}>{error}</Text>}
        {notice && <Text style={[styles.notice, { color: theme.accentDark }]}>{notice}</Text>}

        <Pressable
          testID="login-submit"
          onPress={onSubmit}
          disabled={submitting}
          style={[styles.submit, { backgroundColor: theme.accent, opacity: submitting ? 0.6 : 1 }]}
        >
          <Text style={styles.submitText}>
            {mode === 'login' ? 'Se connecter' : mode === 'register' ? 'Créer un compte' : 'Envoyer le lien'}
          </Text>
        </Pressable>

        {mode === 'login' && (
          <Pressable onPress={() => switchMode('forgot')} hitSlop={8} style={styles.linkButton}>
            <Text style={[styles.linkText, { color: theme.accentDark }]}>Mot de passe oublié ?</Text>
          </Pressable>
        )}
        {mode === 'forgot' && (
          <Pressable onPress={() => switchMode('login')} hitSlop={8} style={styles.linkButton}>
            <Text style={[styles.linkText, { color: theme.accentDark }]}>Retour à la connexion</Text>
          </Pressable>
        )}

        {__DEV__ && mode === 'login' && (
          <Pressable
            onPress={() => {
              setEmail(devSeedEmail);
              setPassword(devSeedPassword);
            }}
            hitSlop={8}
            style={styles.devFillButton}
          >
            <Text style={[styles.devFillText, { color: theme.subtle }]}>Remplir avec le compte de test</Text>
          </Pressable>
        )}
      </View>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, alignItems: 'center', justifyContent: 'center', padding: Spacing.four },
  brand: { alignItems: 'center', gap: Spacing.one, marginBottom: Spacing.four },
  tagline: { fontSize: 15, fontWeight: '500' },
  card: {
    width: '100%',
    maxWidth: 400,
    ...Elevation.raised,
    borderWidth: 1,
    borderRadius: Radius.sheet,
    padding: Spacing.four,
    gap: Spacing.three,
  },
  tabActive: { ...Elevation.card },
  title: { fontSize: 36, fontWeight: '800', letterSpacing: -1.2, marginTop: Spacing.two },
  tabs: { flexDirection: 'row', borderRadius: Radius.pill, padding: 4, gap: 4 },
  tab: { flex: 1, paddingVertical: Spacing.two, borderRadius: Radius.pill, alignItems: 'center' },
  tabText: { fontWeight: '700', fontSize: 13 },
  field: { gap: Spacing.one },
  label: { fontSize: 12, fontWeight: '700' },
  input: { borderWidth: 1, borderRadius: Radius.control, paddingHorizontal: Spacing.three, paddingVertical: 12, fontSize: 15 },
  error: { fontSize: 13 },
  notice: { fontSize: 13, fontWeight: '600' },
  hint: { fontSize: 11.5 },
  linkButton: { alignItems: 'center', paddingVertical: Spacing.one },
  linkText: { fontSize: 13, fontWeight: '700' },
  submit: { ...Elevation.button, borderRadius: Radius.pill, paddingVertical: Spacing.three, alignItems: 'center', marginTop: Spacing.one },
  submitText: { color: '#fff', fontWeight: '700', fontSize: 15 },
  devFillButton: { alignItems: 'center', paddingVertical: Spacing.one },
  devFillText: { fontSize: 12 },
  divider: { flexDirection: 'row', alignItems: 'center', gap: Spacing.two },
  dividerLine: { flex: 1, height: 1 },
  dividerText: { fontSize: 12 },
});
