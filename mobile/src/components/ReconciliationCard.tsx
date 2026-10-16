import { Pressable, StyleSheet, View } from 'react-native';
import { Text } from '@/components/ui/text';
import {
  useReconciliationSuggestions,
  useReconcile,
  useUnreconciledOperations,
} from '@/api/hooks/useReconciliation';
import { Elevation, Radius, Spacing } from '@/constants/theme';
import { formatMoney } from '@/domain/money';
import { useTheme } from '@/hooks/use-theme';

/** Suggested matches between a planned occurrence and a real operation (one
 * tap to confirm), plus the read-only list of operations still unmatched to
 * any occurrence. Ported from frontend/src/pages/account/ReconciliationTab.tsx
 * as a card next to the recurring-suggestions one, in place of that separate
 * tab (no tab navigation for an account yet on mobile). */
export function ReconciliationCard({ accountId, currency }: { accountId: string; currency: string | undefined }) {
  const theme = useTheme();
  const suggestionsQuery = useReconciliationSuggestions(accountId);
  const unreconciledQuery = useUnreconciledOperations(accountId);
  const reconcile = useReconcile(accountId);

  const suggestions = suggestionsQuery.data ?? [];
  const unreconciled = unreconciledQuery.data ?? [];

  if (suggestions.length === 0 && unreconciled.length === 0) return null;

  return (
    <View
      style={[
        styles.card,
        { backgroundColor: theme.surface, borderColor: theme.line },
      ]}
    >
      <Text style={[styles.title, { color: theme.text }]}>Rapprochement</Text>

      {suggestions.length > 0 && (
        <>
          <Text style={[styles.subtitle, { color: theme.textSecondary }]}>
            Correspondances suggérées
          </Text>
          {suggestions.map((s) => (
            <View
              key={s.occurrence.id}
              style={[styles.suggestionRow, { borderTopColor: theme.line }]}
            >
              <View style={styles.rowMain}>
                <Text
                  style={[styles.label, { color: theme.text }]}
                  numberOfLines={1}
                >
                  {s.operation.label}
                </Text>
                <Text style={[styles.meta, { color: theme.textSecondary }]}>
                  {s.operation.date} · prévu {s.occurrence.due_date}
                </Text>
              </View>
              <Text
                style={[
                  styles.amount,
                  {
                    color:
                      Number(s.operation.amount) < 0
                        ? theme.negative
                        : theme.positive,
                  },
                ]}
              >
                {formatMoney(s.operation.amount, currency)}
              </Text>
              <Pressable
                onPress={() =>
                  reconcile.mutate({
                    occurrenceId: s.occurrence.id,
                    operationId: s.operation.id,
                  })
                }
                hitSlop={8}
              >
                <Text
                  style={[styles.confirmAction, { color: theme.accentDark }]}
                >
                  Confirmer
                </Text>
              </Pressable>
            </View>
          ))}
        </>
      )}

      {unreconciled.length > 0 && (
        <>
          <Text
            style={[
              styles.subtitle,
              { color: theme.textSecondary, marginTop: Spacing.three },
            ]}
          >
            Opérations non rapprochées
          </Text>
          {unreconciled.map((op) => (
            <View
              key={op.id}
              style={[styles.suggestionRow, { borderTopColor: theme.line }]}
            >
              <View style={styles.rowMain}>
                <Text
                  style={[styles.label, { color: theme.text }]}
                  numberOfLines={1}
                >
                  {op.label}
                </Text>
                <Text style={[styles.meta, { color: theme.textSecondary }]}>
                  {op.date}
                </Text>
              </View>
              <Text
                style={[
                  styles.amount,
                  {
                    color:
                      Number(op.amount) < 0 ? theme.negative : theme.positive,
                  },
                ]}
              >
                {formatMoney(op.amount, currency)}
              </Text>
            </View>
          ))}
        </>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  card: {
    ...Elevation.card,
    borderRadius: Radius.card,
    padding: Spacing.four,
    marginTop: Spacing.four,
  },
  title: { fontSize: 14, fontWeight: '700' },
  subtitle: {
    fontSize: 11.5,
    fontWeight: '700',
    marginTop: Spacing.two,
    textTransform: 'uppercase',
  },
  suggestionRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: Spacing.three,
    paddingVertical: Spacing.two,
    borderTopWidth: 1,
    marginTop: Spacing.two,
  },
  rowMain: { flex: 1, minWidth: 0 },
  label: { fontSize: 13.5, fontWeight: '600' },
  meta: { fontSize: 11, marginTop: 2 },
  amount: { fontSize: 13, fontWeight: '700', fontVariant: ['tabular-nums'] },
  confirmAction: { fontSize: 12, fontWeight: '700' },
});
