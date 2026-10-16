import { useState } from 'react';
import { Modal, Pressable, ScrollView, StyleSheet, View } from 'react-native';
import { Text, TextInput } from '@/components/ui/text';
import { ApiError } from '@/api/client';
import { useCreatePlannedItem, useGeneratePlannedItemOccurrences } from '@/api/hooks/usePlannedItems';
import type { Category, Operation, PlannedItemKind, RecurrenceType } from '@/api/types';
import { CategoryPicker } from '@/components/CategoryPicker';
import { OptionPicker } from '@/components/OptionPicker';
import { Elevation, Radius, Spacing } from '@/constants/theme';
import { useTheme } from '@/hooks/use-theme';

const KIND_OPTIONS: { value: PlannedItemKind; label: string }[] = [
  { value: 'charge', label: 'Dépense' },
  { value: 'income', label: 'Revenu' },
];

const RECURRENCE_OPTIONS: { value: RecurrenceType; label: string }[] = [
  { value: 'perpetual', label: 'Tous les mois' },
  { value: 'every_x_weeks', label: 'Toutes les X semaines' },
  { value: 'one_off', label: 'Ponctuel' },
];

/** Creates a recurring item prefilled from an existing (usually imported)
 * operation — label, amount, kind and start date all come from it, and
 * optionally a recurrence already guessed for it (see
 * RecurringDetectionUseCase's suggestions, which anchor on a real operation
 * the same way — see suggestionAsOperation in the account screen). Once
 * occurrences are generated, the backend's label-based auto-match
 * reconciles this same operation (and categorizes it) right away. Ported
 * from frontend/src/components/CreateRecurringItemDialog.tsx — mount this
 * with a fresh `key` per operation (see the account screen) so its local
 * state resets instead of carrying over the previous target's edits. */
export function CreateRecurringItemModal({
  accountId,
  operation,
  categories,
  initialRecurrenceType = 'perpetual',
  initialIntervalWeeks = null,
  onClose,
}: {
  accountId: string;
  operation: Operation;
  categories: Category[];
  initialRecurrenceType?: RecurrenceType;
  initialIntervalWeeks?: number | null;
  onClose: () => void;
}) {
  const theme = useTheme();
  const createItem = useCreatePlannedItem();
  const generateOccurrences = useGeneratePlannedItemOccurrences();

  const [label, setLabel] = useState(operation.label);
  const [kind, setKind] = useState<PlannedItemKind>(Number(operation.amount) < 0 ? 'charge' : 'income');
  const [amount, setAmount] = useState(Math.abs(Number(operation.amount)).toFixed(2));
  const [recurrenceType, setRecurrenceType] = useState<RecurrenceType>(initialRecurrenceType);
  const [intervalWeeks, setIntervalWeeks] = useState(initialIntervalWeeks ? String(initialIntervalWeeks) : '1');
  const [categoryId, setCategoryId] = useState<string | null>(operation.category_id);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const dayOfMonth = Number(operation.date.slice(8, 10));

  const onSubmit = async () => {
    setError(null);
    setSubmitting(true);
    try {
      const created = await createItem.mutateAsync({
        accountId,
        kind,
        matchMode: 'label',
        label,
        amount,
        startDate: operation.date,
        recurrenceType,
        occurrenceCount: null,
        intervalWeeks: recurrenceType === 'every_x_weeks' ? Number(intervalWeeks) : null,
        dayOfMonth: recurrenceType === 'perpetual' ? dayOfMonth : null,
        categoryId,
      });
      await generateOccurrences.mutateAsync({ itemId: created.id, monthsAhead: 12 });
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Impossible de créer l'élément récurrent.");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal visible transparent animationType="fade" onRequestClose={onClose}>
      <Pressable style={styles.backdrop} onPress={onClose}>
        <Pressable style={[styles.sheet, { backgroundColor: theme.surface, borderColor: theme.line }]}>
          <ScrollView>
            <Text style={[styles.title, { color: theme.text }]}>Rendre récurrent</Text>

            <View style={styles.field}>
              <Text style={[styles.label, { color: theme.textSecondary }]}>Type</Text>
              <OptionPicker title="Type" placeholder="Type" value={kind} options={KIND_OPTIONS} onChange={setKind} />
            </View>

            <View style={styles.field}>
              <Text style={[styles.label, { color: theme.textSecondary }]}>Libellé</Text>
              <TextInput
                value={label}
                onChangeText={setLabel}
                style={[styles.input, { borderColor: theme.line, color: theme.text }]}
                placeholderTextColor={theme.subtle}
              />
            </View>

            <View style={styles.field}>
              <Text style={[styles.label, { color: theme.textSecondary }]}>Montant</Text>
              <TextInput
                value={amount}
                onChangeText={setAmount}
                keyboardType="numeric"
                style={[styles.input, { borderColor: theme.line, color: theme.text }]}
                placeholderTextColor={theme.subtle}
              />
            </View>

            <View style={styles.field}>
              <Text style={[styles.label, { color: theme.textSecondary }]}>Récurrence</Text>
              <OptionPicker
                title="Récurrence"
                placeholder="Récurrence"
                value={recurrenceType}
                options={RECURRENCE_OPTIONS}
                onChange={setRecurrenceType}
              />
            </View>

            {recurrenceType === 'every_x_weeks' && (
              <View style={styles.field}>
                <Text style={[styles.label, { color: theme.textSecondary }]}>Toutes les combien de semaines</Text>
                <TextInput
                  value={intervalWeeks}
                  onChangeText={setIntervalWeeks}
                  keyboardType="numeric"
                  style={[styles.input, { borderColor: theme.line, color: theme.text }]}
                  placeholderTextColor={theme.subtle}
                />
              </View>
            )}

            <View style={styles.field}>
              <Text style={[styles.label, { color: theme.textSecondary }]}>Catégorie</Text>
              <CategoryPicker categories={categories} value={categoryId} onChange={setCategoryId} />
            </View>

            {error && <Text style={{ color: theme.negative, fontSize: 12.5 }}>{error}</Text>}

            <View style={styles.actions}>
              <Pressable onPress={onClose} hitSlop={8} style={styles.cancelButton}>
                <Text style={[styles.cancelText, { color: theme.textSecondary }]}>Annuler</Text>
              </Pressable>
              <Pressable
                onPress={onSubmit}
                disabled={submitting || !label || !amount}
                style={[styles.submit, { backgroundColor: theme.accent, opacity: submitting || !label || !amount ? 0.6 : 1 }]}
              >
                <Text style={styles.submitText}>{submitting ? 'Création…' : 'Créer'}</Text>
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
  title: { fontSize: 16, fontWeight: '700', marginBottom: Spacing.three },
  field: { gap: 4, marginBottom: Spacing.two },
  label: { fontSize: 11, fontWeight: '700' },
  input: { borderWidth: 1, borderRadius: Radius.control, paddingHorizontal: Spacing.three, paddingVertical: 12, fontSize: 14 },
  actions: { flexDirection: 'row', justifyContent: 'flex-end', gap: Spacing.three, marginTop: Spacing.three },
  cancelButton: { paddingVertical: Spacing.two, paddingHorizontal: Spacing.two },
  cancelText: { fontSize: 13.5, fontWeight: '700' },
  submit: { ...Elevation.button, borderRadius: Radius.pill, paddingVertical: 12, paddingHorizontal: Spacing.four, alignItems: 'center' },
  submitText: { color: '#fff', fontWeight: '700', fontSize: 13.5 },
});
