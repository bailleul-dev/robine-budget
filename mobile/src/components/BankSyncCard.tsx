import { useState } from 'react';
import { ActivityIndicator, Pressable, StyleSheet, View } from 'react-native';
import { Text, TextInput } from '@/components/ui/text';
import { ApiError } from '@/api/client';
import { useBankSyncStatus, useConnectBank, useSyncBank } from '@/api/hooks/useBankSync';
import { Elevation, Radius, Spacing } from '@/constants/theme';
import { useTheme } from '@/hooks/use-theme';

// Only key today is "cm" (see backend's entities.BankSyncBankCreditMutuel) —
// mapped to a display name here rather than in the API so a second bank
// later is just one more entry.
const BANK_NAMES: Record<string, string> = { cm: 'Crédit Mutuel' };

/** One card, two modes depending on GET /bank-sync/status: the connect form
 * (login/password — only Crédit Mutuel is supported, so there's no bank
 * picker, just a fixed label) when nothing is linked yet, or a plain
 * "Synchroniser" action once it is. Error/result state lives here rather
 * than split per mode so it survives the connected-status flip that
 * happens right after a connect attempt (successful or not — see
 * invalidateAfterBankSync). */
export function BankSyncCard() {
  const theme = useTheme();
  const statusQuery = useBankSyncStatus();
  const connect = useConnectBank();
  const sync = useSyncBank();
  const [login, setLogin] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<{ synced: number; created: number } | null>(null);

  const connected = statusQuery.data?.connected ?? false;
  const pending = connect.isPending || sync.isPending;

  const onConnect = async () => {
    setError(null);
    setResult(null);
    try {
      const res = await connect.mutateAsync({ login, password });
      setResult({ synced: res.synced_accounts, created: res.created_accounts });
      setPassword('');
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Connexion impossible.');
    }
  };

  const onSync = async () => {
    setError(null);
    setResult(null);
    try {
      const res = await sync.mutateAsync();
      setResult({ synced: res.synced_accounts, created: res.created_accounts });
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Impossible de synchroniser.');
    }
  };

  return (
    <View style={[styles.card, { backgroundColor: theme.surface, borderColor: theme.line }]}>
      {statusQuery.isLoading && <ActivityIndicator color={theme.accent} />}
      {statusQuery.isError && (
        <Text style={[styles.hint, { color: theme.negative, marginTop: Spacing.two }]}>
          Impossible de charger l&apos;état de la synchronisation.
        </Text>
      )}

      {statusQuery.data && (
        <View style={styles.form}>
          {connected ? (
            <Text style={[styles.hint, { color: theme.textSecondary }]}>
              Compte {BANK_NAMES[statusQuery.data.bank ?? 'cm'] ?? statusQuery.data.bank} connecté.
            </Text>
          ) : (
            <>
              <Text style={[styles.hint, { color: theme.textSecondary }]}>
                Seul le Crédit Mutuel est pris en charge pour l&apos;instant. Vos identifiants sont chiffrés et ne
                servent qu&apos;à récupérer vos soldes.
              </Text>
              <View style={styles.field}>
                <Text style={[styles.label, { color: theme.textSecondary }]}>Banque</Text>
                <View style={[styles.bankBadge, { backgroundColor: theme.surfaceSoft }]}>
                  <Text style={{ color: theme.text, fontWeight: '700', fontSize: 13 }}>Crédit Mutuel</Text>
                </View>
              </View>
              <View style={styles.field}>
                <Text style={[styles.label, { color: theme.textSecondary }]}>Identifiant</Text>
                <TextInput
                  value={login}
                  onChangeText={setLogin}
                  autoCapitalize="none"
                  autoComplete="username"
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
                  autoComplete="current-password"
                  style={[styles.input, { borderColor: theme.line, color: theme.text }]}
                  placeholderTextColor={theme.subtle}
                />
              </View>
            </>
          )}

          {connect.isPending && (
            <Text style={[styles.hint, { color: theme.forecastText }]}>
              Validation en attente dans votre application bancaire…
            </Text>
          )}
          {error && <Text style={{ color: theme.negative, fontSize: 12.5 }}>{error}</Text>}
          {result && (
            <Text style={{ color: theme.positive, fontSize: 12.5 }}>
              {result.synced} compte{result.synced > 1 ? 's' : ''} synchronisé{result.synced > 1 ? 's' : ''}
              {result.created > 0
                ? ` (dont ${result.created} nouveau${result.created > 1 ? 'x' : ''})`
                : ''}
              .
            </Text>
          )}

          <Pressable
            onPress={connected ? onSync : onConnect}
            disabled={pending || (!connected && (!login || !password))}
            style={[
              styles.submit,
              { backgroundColor: theme.accent, opacity: pending || (!connected && (!login || !password)) ? 0.6 : 1 },
            ]}
          >
            <Text style={styles.submitText}>
              {connected
                ? sync.isPending
                  ? 'Synchronisation…'
                  : 'Synchroniser maintenant'
                : connect.isPending
                  ? 'Connexion…'
                  : 'Connecter'}
            </Text>
          </Pressable>
        </View>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  card: { ...Elevation.card, borderRadius: Radius.card, padding: Spacing.four },
  hint: { fontSize: 12.5, lineHeight: 17 },
  form: { gap: Spacing.two },
  field: { gap: 4 },
  label: { fontSize: 11, fontWeight: '700' },
  bankBadge: { borderRadius: 16, paddingHorizontal: Spacing.three, paddingVertical: Spacing.two, alignSelf: 'flex-start' },
  input: { borderWidth: 1, borderRadius: Radius.control, paddingHorizontal: Spacing.three, paddingVertical: 12, fontSize: 14 },
  submit: { ...Elevation.button, borderRadius: Radius.pill, paddingVertical: 12, alignItems: 'center', marginTop: 4 },
  submitText: { color: '#fff', fontWeight: '700', fontSize: 13.5 },
});
