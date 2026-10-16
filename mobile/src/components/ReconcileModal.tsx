import { useQueryClient } from '@tanstack/react-query';
import { ActivityIndicator, Modal, Pressable, ScrollView, StyleSheet } from 'react-native';
import { Text } from '@/components/ui/text';
import { dashboardKey } from '@/api/hooks/useDashboard';
import {
  unreconciledOperationsKey,
  useReconcile,
  useUnreconciledOperations,
} from '@/api/hooks/useReconciliation';
import type { MonthlyOccurrence } from '@/domain/dashboard';
import { formatMoney } from '@/domain/money';
import { Elevation, Radius, Spacing } from '@/constants/theme';
import { useTheme } from '@/hooks/use-theme';

/** Picks which real operation a forecast occurrence corresponds to — opened
 * by tapping a "Prévu" line in the dashboard's monthly occurrences list.
 * Ported from frontend/src/pages/OverviewScreen.tsx's ReconcileDialog. */
export function ReconcileModal({
  occurrence,
  onClose,
}: {
  occurrence: MonthlyOccurrence;
  onClose: () => void;
}) {
  const theme = useTheme();
  const queryClient = useQueryClient();
  const operationsQuery = useUnreconciledOperations(occurrence.accountId);
  const reconcile = useReconcile(occurrence.accountId);

  const onPick = async (operationId: string) => {
    await reconcile.mutateAsync({
      occurrenceId: occurrence.occurrenceId,
      operationId,
    });
    queryClient.invalidateQueries({ queryKey: dashboardKey });
    queryClient.invalidateQueries({
      queryKey: unreconciledOperationsKey(occurrence.accountId),
    });
    onClose();
  };

  return (
    <Modal visible transparent animationType="fade" onRequestClose={onClose}>
      <Pressable style={styles.backdrop} onPress={onClose}>
        <Pressable
          style={[
            styles.sheet,
            { backgroundColor: theme.surface, borderColor: theme.line },
          ]}
        >
          <Text style={[styles.title, { color: theme.text }]} numberOfLines={2}>
            Rapprocher « {occurrence.label} »
          </Text>

          <ScrollView style={styles.list}>
            {operationsQuery.isLoading && (
              <ActivityIndicator
                color={theme.accent}
                style={{ marginTop: Spacing.four }}
              />
            )}
            {operationsQuery.data?.length === 0 && (
              <Text
                style={{
                  color: theme.textSecondary,
                  paddingVertical: Spacing.three,
                }}
              >
                Aucune opération non rapprochée.
              </Text>
            )}
            {operationsQuery.data?.map((op) => (
              <Pressable
                key={op.id}
                onPress={() => onPick(op.id)}
                style={[styles.row, { borderTopColor: theme.line }]}
              >
                <Text
                  style={[styles.rowLabel, { color: theme.text }]}
                  numberOfLines={1}
                >
                  {op.label}
                </Text>
                <Text style={[styles.rowMeta, { color: theme.textSecondary }]}>
                  {op.date} · {formatMoney(op.amount, occurrence.currency)}
                </Text>
              </Pressable>
            ))}
          </ScrollView>

          <Pressable onPress={onClose} hitSlop={8} style={styles.closeButton}>
            <Text
              style={[styles.closeButtonText, { color: theme.textSecondary }]}
            >
              Fermer
            </Text>
          </Pressable>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    flex: 1,
    backgroundColor: 'rgba(13, 24, 32, 0.45)',
    justifyContent: 'center',
    padding: Spacing.four,
  },
  sheet: {
    ...Elevation.raised,
    borderRadius: Radius.sheet,
    maxHeight: '75%',
    padding: Spacing.four,
  },
  title: { fontSize: 15, fontWeight: '700', marginBottom: Spacing.two },
  list: { flexGrow: 0 },
  row: { paddingVertical: Spacing.three, borderTopWidth: 1 },
  rowLabel: { fontSize: 13.5, fontWeight: '600' },
  rowMeta: { fontSize: 11.5, marginTop: 2 },
  closeButton: { alignSelf: 'center', marginTop: Spacing.three },
  closeButtonText: { fontSize: 13, fontWeight: '600' },
});
