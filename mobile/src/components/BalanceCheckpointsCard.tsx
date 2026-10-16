import { useState } from 'react';
import { Pressable, StyleSheet, View } from 'react-native';
import { Text, TextInput } from '@/components/ui/text';
import { ApiError } from '@/api/client';
import {
  useBalanceCheckpoints,
  useComputeInitialBalance,
  useCreateBalanceCheckpoint,
  useDeleteBalanceCheckpoint,
} from '@/api/hooks/useBalanceCheckpoints';
import { Elevation, Radius, Spacing } from '@/constants/theme';
import { formatMoney } from '@/domain/money';
import { useTheme } from '@/hooks/use-theme';

function todayIso(): string {
  return new Date().toISOString().slice(0, 10);
}

/** Ported from frontend/src/pages/account/PrevisionsTab.tsx's
 * BalanceCheckpointsCard: while the account has no checkpoint yet, offer
 * the simpler "compute it for me" bootstrap (ComputeInitialBalanceUseCase)
 * instead of the generic "add a checkpoint dated X" form — there's nothing
 * yet to add a correction relative to. Read-only for a shared account
 * (isOwner false): checkpoints affect the real bank-balance anchor, out of
 * scope for a collaborator (see backend's RequireOwnAccount). */
export function BalanceCheckpointsCard({
  accountId,
  currency,
  isOwner,
}: {
  accountId: string;
  currency: string | undefined;
  isOwner: boolean;
}) {
  const theme = useTheme();
  const checkpointsQuery = useBalanceCheckpoints(accountId);
  const deleteCheckpoint = useDeleteBalanceCheckpoint(accountId);

  const checkpoints = checkpointsQuery.data ?? [];
  const hasCheckpoint = checkpoints.length > 0;

  return (
    <View style={[styles.card, { backgroundColor: theme.surface, borderColor: theme.line }]}>
      <Text style={[styles.title, { color: theme.text }]}>Solde de référence</Text>
      <Text style={[styles.hint, { color: theme.textSecondary }]}>
        {hasCheckpoint
          ? 'Indiquez le solde réel du compte à une date donnée pour que le solde actuel et les prévisions restent exacts.'
          : "Indiquez votre solde réel actuel — le solde de départ sera calculé automatiquement à partir de vos opérations importées."}
      </Text>

      {checkpoints.map((c, i) => (
        <View key={c.id} style={[styles.row, i === 0 && { marginTop: Spacing.three }, { borderTopColor: theme.line }]}>
          <View style={styles.rowMain}>
            <Text style={[styles.rowDate, { color: theme.text }]}>{c.as_of_date}</Text>
            {c.note && <Text style={[styles.rowNote, { color: theme.textSecondary }]}>{c.note}</Text>}
          </View>
          <Text style={[styles.rowBalance, { color: theme.text }]}>{formatMoney(c.balance, currency)}</Text>
          {isOwner && (
            <Pressable onPress={() => deleteCheckpoint.mutate(c.id)} hitSlop={8}>
              <Text style={{ color: theme.negative, fontSize: 12, fontWeight: '700' }}>Suppr.</Text>
            </Pressable>
          )}
        </View>
      ))}

      {isOwner && (hasCheckpoint ? <AddCheckpointForm accountId={accountId} /> : <InitialBalanceForm accountId={accountId} />)}
    </View>
  );
}

function AddCheckpointForm({ accountId }: { accountId: string }) {
  const theme = useTheme();
  const createCheckpoint = useCreateBalanceCheckpoint(accountId);
  const [asOfDate, setAsOfDate] = useState(todayIso());
  const [balance, setBalance] = useState('');
  const [error, setError] = useState<string | null>(null);

  const onSubmit = async () => {
    setError(null);
    try {
      await createCheckpoint.mutateAsync({ asOfDate, balance, note: null });
      setBalance('');
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Impossible d'enregistrer le solde.");
    }
  };

  return (
    <View style={styles.form}>
      <CheckpointFields
        dateLabel="Date"
        balanceLabel="Solde"
        asOfDate={asOfDate}
        setAsOfDate={setAsOfDate}
        balance={balance}
        setBalance={setBalance}
        theme={theme}
      />
      {error && <Text style={{ color: theme.negative, fontSize: 12.5 }}>{error}</Text>}
      <Pressable
        onPress={onSubmit}
        disabled={createCheckpoint.isPending || !balance}
        style={[styles.submit, { backgroundColor: theme.accent, opacity: createCheckpoint.isPending || !balance ? 0.6 : 1 }]}
      >
        <Text style={styles.submitText}>Enregistrer le solde</Text>
      </Pressable>
    </View>
  );
}

function InitialBalanceForm({ accountId }: { accountId: string }) {
  const theme = useTheme();
  const computeInitialBalance = useComputeInitialBalance(accountId);
  const [asOfDate, setAsOfDate] = useState(todayIso());
  const [balance, setBalance] = useState('');
  const [error, setError] = useState<string | null>(null);

  const onSubmit = async () => {
    setError(null);
    try {
      await computeInitialBalance.mutateAsync({ asOfDate, balance, note: null });
      setBalance('');
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Impossible d'enregistrer le solde.");
    }
  };

  return (
    <View style={styles.form}>
      <CheckpointFields
        dateLabel="Solde connu à la date du"
        balanceLabel="Solde actuel"
        asOfDate={asOfDate}
        setAsOfDate={setAsOfDate}
        balance={balance}
        setBalance={setBalance}
        theme={theme}
      />
      {error && <Text style={{ color: theme.negative, fontSize: 12.5 }}>{error}</Text>}
      <Pressable
        onPress={onSubmit}
        disabled={computeInitialBalance.isPending || !balance}
        style={[
          styles.submit,
          { backgroundColor: theme.accent, opacity: computeInitialBalance.isPending || !balance ? 0.6 : 1 },
        ]}
      >
        <Text style={styles.submitText}>Calculer le solde initial</Text>
      </Pressable>
    </View>
  );
}

// Shared date/balance inputs for both forms above — a plain "YYYY-MM-DD"
// text field for now rather than a native date picker (no extra dependency
// for this first pass; RN has no built-in <input type="date">).
function CheckpointFields({
  dateLabel,
  balanceLabel,
  asOfDate,
  setAsOfDate,
  balance,
  setBalance,
  theme,
}: {
  dateLabel: string;
  balanceLabel: string;
  asOfDate: string;
  setAsOfDate: (v: string) => void;
  balance: string;
  setBalance: (v: string) => void;
  theme: ReturnType<typeof useTheme>;
}) {
  return (
    <View style={styles.fieldsRow}>
      <View style={styles.field}>
        <Text style={[styles.label, { color: theme.textSecondary }]}>{dateLabel}</Text>
        <TextInput
          value={asOfDate}
          onChangeText={setAsOfDate}
          placeholder="AAAA-MM-JJ"
          placeholderTextColor={theme.subtle}
          style={[styles.input, { borderColor: theme.line, color: theme.text }]}
        />
      </View>
      <View style={styles.field}>
        <Text style={[styles.label, { color: theme.textSecondary }]}>{balanceLabel}</Text>
        <TextInput
          value={balance}
          onChangeText={setBalance}
          placeholder="0.00"
          placeholderTextColor={theme.subtle}
          keyboardType="numeric"
          style={[styles.input, { borderColor: theme.line, color: theme.text }]}
        />
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  card: { ...Elevation.card, borderRadius: Radius.card, padding: Spacing.four },
  title: { fontSize: 15, fontWeight: '700' },
  hint: { marginTop: Spacing.one, fontSize: 12.5, lineHeight: 17 },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: Spacing.three,
    paddingVertical: Spacing.two,
    borderTopWidth: 1,
  },
  rowMain: { flex: 1, minWidth: 0 },
  rowDate: { fontSize: 13, fontWeight: '600' },
  rowNote: { fontSize: 11.5, marginTop: 1 },
  rowBalance: { fontSize: 13, fontWeight: '700', fontVariant: ['tabular-nums'] },
  form: { marginTop: Spacing.three, gap: Spacing.two },
  fieldsRow: { flexDirection: 'row', gap: Spacing.three },
  field: { flex: 1, gap: 4 },
  label: { fontSize: 11, fontWeight: '700' },
  input: { borderWidth: 1, borderRadius: Radius.control, paddingHorizontal: Spacing.three, paddingVertical: 12, fontSize: 14 },
  submit: { ...Elevation.button, borderRadius: Radius.pill, paddingVertical: 12, alignItems: 'center', marginTop: 4 },
  submitText: { color: '#fff', fontWeight: '700', fontSize: 13.5 },
});
