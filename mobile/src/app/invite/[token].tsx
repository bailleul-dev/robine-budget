import { useEffect, useState } from 'react';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { ActivityIndicator, KeyboardAvoidingView, Platform, Pressable, StyleSheet, View } from 'react-native';
import { Text, TextInput } from '@/components/ui/text';
import { ApiError } from '@/api/client';
import { useAuthConfig } from '@/api/hooks/useAuthConfig';
import { useInviteInfo, useRedeemInvite } from '@/api/hooks/useCollaborators';
import { useAuth } from '@/auth/AuthContext';
import { googleAuthConfigured } from '@/auth/google';
import { GoogleSignInButton } from '@/components/GoogleSignInButton';
import { RobineSignInButton } from '@/components/RobineSignInButton';
import { Elevation, Radius, Spacing } from '@/constants/theme';
import { useTheme } from '@/hooks/use-theme';

/** Landing page for a share link (e.g. "/invite/abc123"). A first-time
 * visitor signs in here (Google is the expected path — no account needed
 * beforehand) and is immediately granted access to the inviting user's
 * accounts; from then on they just use the normal login screen to come
 * back, so this page only matters for the initial handoff. Ported from
 * frontend/src/pages/InvitePage.tsx — same states, same auto-redeem-on-
 * sign-in behavior (GoogleSignInButton already updates the shared `user`
 * via AuthContext, so the effect below fires the same way regardless of
 * whether sign-in happened through Google or the form). Reachable
 * regardless of auth state — see the root layout's always-visible
 * Stack.Screen for this route. */
export default function InviteScreen() {
  const { token } = useLocalSearchParams<{ token: string }>();
  const theme = useTheme();
  const router = useRouter();
  const { user, loading: authLoading, login, register, logout } = useAuth();
  const inviteQuery = useInviteInfo(token ?? '');
  const redeemInvite = useRedeemInvite();
  const authConfigQuery = useAuthConfig();
  const robineAuthConfigured = authConfigQuery.data?.robine_enabled ?? false;

  const [mode, setMode] = useState<'login' | 'register'>('login');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [formError, setFormError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [redeemState, setRedeemState] = useState<'idle' | 'success' | 'error'>('idle');
  const [redeemError, setRedeemError] = useState<string | null>(null);
  const [ownerName, setOwnerName] = useState<string | null>(null);

  // Once signed in (via Google, or the fallback form below), claim the
  // invite right away — no extra tap needed.
  useEffect(() => {
    if (!user || redeemState !== 'idle' || !token) return;
    redeemInvite.mutate(token, {
      onSuccess: (res) => {
        setOwnerName(res.owner_display_name);
        setRedeemState('success');
      },
      onError: (err) => {
        setRedeemError(err instanceof ApiError ? err.message : 'Une erreur est survenue.');
        setRedeemState('error');
      },
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [user, token]);

  const onSubmit = async () => {
    setFormError(null);
    setSubmitting(true);
    try {
      if (mode === 'login') {
        await login(email, password);
      } else {
        await register(email, password, displayName);
      }
    } catch (err) {
      setFormError(err instanceof ApiError ? err.message : 'Une erreur est survenue.');
    } finally {
      setSubmitting(false);
    }
  };

  if (inviteQuery.isLoading || authLoading) {
    return (
      <View style={[styles.screen, { backgroundColor: theme.background }]}>
        <ActivityIndicator color={theme.accent} />
      </View>
    );
  }

  if (inviteQuery.isError) {
    return (
      <View style={[styles.screen, { backgroundColor: theme.background }]}>
        <View style={[styles.card, { backgroundColor: theme.surface, borderColor: theme.line }]}>
          <Text style={[styles.title, { color: theme.text }]}>Budget</Text>
          <Text style={[styles.message, { color: theme.negative }]}>Ce lien d&apos;invitation n&apos;existe pas.</Text>
        </View>
      </View>
    );
  }

  const info = inviteQuery.data!;

  if (info.status === 'revoked') {
    return (
      <View style={[styles.screen, { backgroundColor: theme.background }]}>
        <View style={[styles.card, { backgroundColor: theme.surface, borderColor: theme.line }]}>
          <Text style={[styles.title, { color: theme.text }]}>Budget</Text>
          <Text style={[styles.message, { color: theme.negative }]}>Ce lien d&apos;invitation n&apos;est plus valide.</Text>
        </View>
      </View>
    );
  }

  if (redeemState === 'success') {
    return (
      <View style={[styles.screen, { backgroundColor: theme.background }]}>
        <View style={[styles.card, { backgroundColor: theme.surface, borderColor: theme.line }]}>
          <Text style={[styles.title, { color: theme.text }]}>Budget</Text>
          <Text style={[styles.message, { color: theme.positive }]}>
            Vous avez maintenant accès aux comptes de {ownerName}.
          </Text>
          <Pressable
            onPress={() => router.replace('/')}
            style={[styles.submit, { backgroundColor: theme.accent }]}
          >
            <Text style={styles.submitText}>Accéder au tableau de bord</Text>
          </Pressable>
        </View>
      </View>
    );
  }

  if (redeemState === 'error') {
    return (
      <View style={[styles.screen, { backgroundColor: theme.background }]}>
        <View style={[styles.card, { backgroundColor: theme.surface, borderColor: theme.line }]}>
          <Text style={[styles.title, { color: theme.text }]}>Budget</Text>
          <Text style={[styles.message, { color: theme.negative }]}>{redeemError}</Text>
          {/* The most common cause is being signed in with the wrong
           * account for this particular invite (each is tied to one email
           * address) — offer a quick way to switch. */}
          <Pressable
            onPress={async () => {
              await logout();
              setRedeemState('idle');
              setRedeemError(null);
            }}
            style={[styles.submit, { backgroundColor: theme.accent }]}
          >
            <Text style={styles.submitText}>Se déconnecter et réessayer</Text>
          </Pressable>
          <Pressable onPress={() => router.replace('/')} style={styles.secondaryButton}>
            <Text style={[styles.secondaryButtonText, { color: theme.textSecondary }]}>Accéder au tableau de bord</Text>
          </Pressable>
        </View>
      </View>
    );
  }

  if (user) {
    // Signed in but the auto-redeem effect hasn't resolved yet.
    return (
      <View style={[styles.screen, { backgroundColor: theme.background }]}>
        <ActivityIndicator color={theme.accent} />
      </View>
    );
  }

  return (
    <KeyboardAvoidingView
      behavior={Platform.OS === 'ios' ? 'padding' : undefined}
      style={[styles.screen, { backgroundColor: theme.background }]}
    >
      <View style={[styles.card, { backgroundColor: theme.surface, borderColor: theme.line }]}>
        <Text style={[styles.title, { color: theme.text }]}>Budget</Text>
        <Text style={[styles.invitedBy, { color: theme.textSecondary }]}>
          {info.owner_display_name} vous invite à accéder à son budget.
        </Text>

        {(googleAuthConfigured || robineAuthConfigured) && (
          <>
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
            style={[styles.tab, mode === 'login' && { backgroundColor: theme.surface }]}
            onPress={() => setMode('login')}
          >
            <Text style={[styles.tabText, { color: mode === 'login' ? theme.accentDark : theme.textSecondary }]}>
              Se connecter
            </Text>
          </Pressable>
          <Pressable
            style={[styles.tab, mode === 'register' && { backgroundColor: theme.surface }]}
            onPress={() => setMode('register')}
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
              style={[styles.input, { borderColor: theme.line, color: theme.text }]}
              placeholderTextColor={theme.subtle}
            />
          </View>
        )}
        <View style={styles.field}>
          <Text style={[styles.label, { color: theme.textSecondary }]}>E-mail</Text>
          <TextInput
            value={email}
            onChangeText={setEmail}
            autoCapitalize="none"
            keyboardType="email-address"
            autoComplete="email"
            style={[styles.input, { borderColor: theme.line, color: theme.text }]}
            placeholderTextColor={theme.subtle}
          />
        </View>
        <View style={styles.field}>
          <Text style={[styles.label, { color: theme.textSecondary }]}>Mot de passe</Text>
          <TextInput
            value={password}
            onChangeText={setPassword}
            secureTextEntry
            autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
            style={[styles.input, { borderColor: theme.line, color: theme.text }]}
            placeholderTextColor={theme.subtle}
          />
        </View>

        {formError && <Text style={[styles.message, { color: theme.negative }]}>{formError}</Text>}

        <Pressable
          onPress={onSubmit}
          disabled={submitting}
          style={[styles.submit, { backgroundColor: theme.accent, opacity: submitting ? 0.6 : 1 }]}
        >
          <Text style={styles.submitText}>{mode === 'login' ? 'Se connecter' : 'Créer un compte'}</Text>
        </Pressable>
      </View>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, alignItems: 'center', justifyContent: 'center', padding: Spacing.four },
  card: { width: '100%', maxWidth: 380, ...Elevation.card, borderRadius: Radius.card, padding: Spacing.five, gap: Spacing.three },
  title: { fontSize: 28, fontWeight: '800', letterSpacing: -0.3, marginBottom: Spacing.two },
  invitedBy: { fontSize: 13.5, marginTop: -Spacing.two },
  message: { fontSize: 14 },
  divider: { flexDirection: 'row', alignItems: 'center', gap: Spacing.two },
  dividerLine: { flex: 1, height: 1 },
  dividerText: { fontSize: 12 },
  tabs: { flexDirection: 'row', borderRadius: Radius.pill, padding: 4, gap: 4 },
  tab: { flex: 1, paddingVertical: Spacing.two, borderRadius: Radius.pill, alignItems: 'center' },
  tabText: { fontWeight: '700', fontSize: 13 },
  field: { gap: Spacing.one },
  label: { fontSize: 12, fontWeight: '700' },
  input: { borderWidth: 1, borderRadius: Radius.control, paddingHorizontal: Spacing.three, paddingVertical: 12, fontSize: 15 },
  submit: { ...Elevation.button, borderRadius: Radius.pill, paddingVertical: Spacing.three, alignItems: 'center', marginTop: Spacing.one },
  submitText: { color: '#fff', fontWeight: '700', fontSize: 15 },
  secondaryButton: { alignItems: 'center', paddingVertical: Spacing.two },
  secondaryButtonText: { fontWeight: '700', fontSize: 13.5 },
});
