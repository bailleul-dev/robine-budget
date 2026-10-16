import { useState } from 'react';
import { Pressable, StyleSheet, View } from 'react-native';
import { Text } from '@/components/ui/text';
import { api, ApiError } from '@/api/client';
import { useAuth } from '@/auth/AuthContext';
import { Radius, Spacing } from '@/constants/theme';
import { useTheme } from '@/hooks/use-theme';

/** Shown under the account card while the user's address is unconfirmed.
 * Confirming happens in the browser (the link in the email opens a small
 * server-rendered page), so this offers two actions: send the email again,
 * and re-check once they've clicked it. Sharing invites are refused
 * server-side until the address is confirmed. */
export function EmailVerificationNotice() {
  const theme = useTheme();
  const { user, refreshUser } = useAuth();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);

  if (!user || !user.email || user.email_verified) return null;

  const run = async (action: () => Promise<string>) => {
    setBusy(true);
    setMessage(null);
    try {
      setMessage(await action());
    } catch (err) {
      setMessage(err instanceof ApiError ? err.message : 'Une erreur est survenue.');
    } finally {
      setBusy(false);
    }
  };

  const resend = () =>
    run(async () => {
      await api.post('/auth/resend-verification');
      return 'E-mail envoyé. Pensez à vérifier vos courriers indésirables.';
    });

  const recheck = () =>
    run(async () => {
      await refreshUser();
      return 'Pas encore confirmé — ouvrez le lien reçu par e-mail, puis revenez ici.';
    });

  return (
    <View style={[styles.box, { backgroundColor: theme.forecastBg, borderColor: theme.line }]}>
      <Text style={[styles.title, { color: theme.forecastText }]}>Confirmez votre adresse e-mail</Text>
      <Text style={[styles.body, { color: theme.textSecondary }]}>
        Nous avons envoyé un lien à {user.email}. Il est nécessaire pour accepter un partage de compte.
      </Text>
      <View style={styles.actions}>
        <Pressable onPress={resend} disabled={busy} hitSlop={8}>
          <Text style={[styles.action, { color: theme.accentDark, opacity: busy ? 0.6 : 1 }]}>Renvoyer l&apos;e-mail</Text>
        </Pressable>
        <Pressable onPress={recheck} disabled={busy} hitSlop={8}>
          <Text style={[styles.action, { color: theme.accentDark, opacity: busy ? 0.6 : 1 }]}>J&apos;ai confirmé</Text>
        </Pressable>
      </View>
      {message && <Text style={[styles.body, { color: theme.textSecondary }]}>{message}</Text>}
    </View>
  );
}

const styles = StyleSheet.create({
  box: { borderWidth: 1, borderRadius: Radius.card, padding: Spacing.three, gap: Spacing.two, marginTop: Spacing.three },
  title: { fontSize: 14, fontWeight: '800' },
  body: { fontSize: 12.5, lineHeight: 17 },
  actions: { flexDirection: 'row', gap: Spacing.four },
  action: { fontSize: 12.5, fontWeight: '700' },
});
