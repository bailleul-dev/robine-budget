import { useState } from 'react';
import { Modal, Pressable, ScrollView, StyleSheet, View } from 'react-native';
import { Text, TextInput } from '@/components/ui/text';
import { ApiError } from '@/api/client';
import { useSetOperationSplits } from '@/api/hooks/useOperations';
import type { Category, Operation } from '@/api/types';
import { CategoryPicker } from '@/components/CategoryPicker';
import { Elevation, Radius, Spacing } from '@/constants/theme';
import { formatMoney } from '@/domain/money';
import { useTheme } from '@/hooks/use-theme';

type Row = { categoryId: string | null; amount: string };

/** Divides one operation's amount across several categories — e.g. a 100€
 * supermarket run split into 20€ groceries + 80€ clothing. Prefills from
 * the operation's existing splits, or two empty rows the first time.
 * Submitting an empty/all-blank set of rows clears the split back to a
 * single (plain) category via the normal picker. Ported from
 * frontend/src/components/SplitOperationDialog.tsx. */
export function SplitOperationModal({
  accountId,
  currency,
  operation,
  categories,
  onClose,
}: {
  accountId: string;
  currency: string | undefined;
  operation: Operation;
  categories: Category[];
  onClose: () => void;
}) {
  const theme = useTheme();
  const setSplits = useSetOperationSplits(accountId);
  const totalAmount = Math.abs(Number(operation.amount));

  const [rows, setRows] = useState<Row[]>(
    operation.splits.length > 0
      ? operation.splits.map((s) => ({ categoryId: s.category_id, amount: s.amount }))
      : [
          { categoryId: null, amount: '' },
          { categoryId: null, amount: '' },
        ],
  );
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const updateRow = (index: number, patch: Partial<Row>) =>
    setRows((prev) => prev.map((row, i) => (i === index ? { ...row, ...patch } : row)));

  const removeRow = (index: number) => setRows((prev) => prev.filter((_, i) => i !== index));

  const addRow = () => setRows((prev) => [...prev, { categoryId: null, amount: '' }]);

  const sum = rows.reduce((acc, row) => acc + (Number(row.amount) || 0), 0);
  const remaining = totalAmount - sum;

  const onSubmit = async () => {
    setError(null);
    const filled = rows.filter((row) => row.categoryId && row.amount);
    if (filled.some((row) => !(Number(row.amount) > 0))) {
      setError('Chaque montant doit être un nombre positif.');
      return;
    }
    if (filled.length === 1) {
      setError('Il faut au moins deux catégories pour diviser une opération.');
      return;
    }
    if (filled.length > 0 && Math.abs(sum - totalAmount) > 0.005) {
      setError(`La somme des montants doit être égale à ${totalAmount.toFixed(2)}.`);
      return;
    }
    setSubmitting(true);
    try {
      await setSplits.mutateAsync({
        operationId: operation.id,
        splits: filled.map((row) => ({ categoryId: row.categoryId as string, amount: row.amount })),
      });
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Impossible d'enregistrer la répartition.");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal visible transparent animationType="fade" onRequestClose={onClose}>
      <Pressable style={styles.backdrop} onPress={onClose}>
        <Pressable style={[styles.sheet, { backgroundColor: theme.surface, borderColor: theme.line }]}>
          <ScrollView>
            <Text style={[styles.title, { color: theme.text }]} numberOfLines={2}>
              Diviser « {operation.label} »
            </Text>
            <Text style={[styles.hint, { color: theme.textSecondary }]}>
              Répartissez les {formatMoney(totalAmount, currency)} de cette opération entre plusieurs catégories.
            </Text>

            <View style={styles.rows}>
              {rows.map((row, i) => (
                <View key={i} style={styles.row}>
                  <View style={styles.rowCategory}>
                    <CategoryPicker
                      categories={categories}
                      value={row.categoryId}
                      onChange={(categoryId) => updateRow(i, { categoryId })}
                    />
                  </View>
                  <TextInput
                    value={row.amount}
                    onChangeText={(amount) => updateRow(i, { amount })}
                    placeholder="0.00"
                    keyboardType="numeric"
                    style={[styles.amountInput, { borderColor: theme.line, color: theme.text }]}
                    placeholderTextColor={theme.subtle}
                  />
                  <Pressable onPress={() => removeRow(i)} disabled={rows.length <= 1} hitSlop={8}>
                    <Text style={{ color: rows.length <= 1 ? theme.subtle : theme.negative, fontSize: 16, fontWeight: '700' }}>
                      ✕
                    </Text>
                  </Pressable>
                </View>
              ))}
            </View>

            <Pressable onPress={addRow} hitSlop={8} style={styles.addRow}>
              <Text style={[styles.addRowText, { color: theme.accentDark }]}>+ Ajouter une catégorie</Text>
            </Pressable>

            <Text style={{ color: Math.abs(remaining) > 0.005 ? theme.negative : theme.textSecondary, fontSize: 12.5, marginTop: Spacing.two }}>
              Reste à répartir : {remaining.toFixed(2)}
            </Text>
            {error && <Text style={{ color: theme.negative, fontSize: 12.5, marginTop: Spacing.one }}>{error}</Text>}

            <View style={styles.actions}>
              <Pressable onPress={onClose} hitSlop={8} style={styles.cancelButton}>
                <Text style={[styles.cancelText, { color: theme.textSecondary }]}>Annuler</Text>
              </Pressable>
              <Pressable
                onPress={onSubmit}
                disabled={submitting}
                style={[styles.submit, { backgroundColor: theme.accent, opacity: submitting ? 0.6 : 1 }]}
              >
                <Text style={styles.submitText}>{submitting ? 'Enregistrement…' : 'Enregistrer la répartition'}</Text>
              </Pressable>
            </View>
          </ScrollView>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, backgroundColor: 'rgba(13, 24, 32, 0.45)', justifyContent: 'center', padding: Spacing.four },
  sheet: { ...Elevation.raised, borderRadius: Radius.sheet, maxHeight: '85%', padding: Spacing.four },
  title: { fontSize: 16, fontWeight: '700' },
  hint: { fontSize: 12.5, marginTop: 4, marginBottom: Spacing.three },
  rows: { gap: Spacing.two },
  row: { flexDirection: 'row', alignItems: 'center', gap: Spacing.two },
  rowCategory: { flex: 1 },
  amountInput: { width: 80, borderWidth: 1, borderRadius: 16, paddingHorizontal: Spacing.two, paddingVertical: Spacing.two, fontSize: 14 },
  addRow: { marginTop: Spacing.two },
  addRowText: { fontSize: 12.5, fontWeight: '700' },
  actions: { flexDirection: 'row', justifyContent: 'flex-end', gap: Spacing.three, marginTop: Spacing.three },
  cancelButton: { paddingVertical: Spacing.two, paddingHorizontal: Spacing.two },
  cancelText: { fontSize: 13.5, fontWeight: '700' },
  submit: { ...Elevation.button, borderRadius: Radius.pill, paddingVertical: 12, paddingHorizontal: Spacing.four, alignItems: 'center' },
  submitText: { color: '#fff', fontWeight: '700', fontSize: 13.5 },
});
