import { useState } from 'react';
import { Modal, Pressable, StyleSheet, View } from 'react-native';
import { Text, TextInput } from '@/components/ui/text';
import { Elevation, Radius, Spacing } from '@/constants/theme';
import { useTheme } from '@/hooks/use-theme';

/** A single-text-field prompt — "add subcategory", "rename category".
 * RN's Alert.prompt only exists on iOS (not Android, not web), so this is
 * a small modal instead — mirrors frontend/src/components/PromptDialog.tsx.
 * Mount with a fresh `key` per target so `initialValue` resets correctly
 * (a mounted component's initial useState value doesn't re-run on
 * prop changes). */
export function PromptModal({
  title,
  initialValue = '',
  submitLabel = 'Enregistrer',
  onSubmit,
  onClose,
}: {
  title: string;
  initialValue?: string;
  submitLabel?: string;
  onSubmit: (value: string) => void;
  onClose: () => void;
}) {
  const theme = useTheme();
  const [value, setValue] = useState(initialValue);

  const submit = () => {
    const trimmed = value.trim();
    if (!trimmed) return;
    onSubmit(trimmed);
  };

  return (
    <Modal visible transparent animationType="fade" onRequestClose={onClose}>
      <Pressable style={styles.backdrop} onPress={onClose}>
        <Pressable style={[styles.sheet, { backgroundColor: theme.surface, borderColor: theme.line }]}>
          <Text style={[styles.title, { color: theme.text }]}>{title}</Text>
          <TextInput
            value={value}
            onChangeText={setValue}
            autoFocus
            style={[styles.input, { borderColor: theme.line, color: theme.text }]}
            placeholderTextColor={theme.subtle}
          />
          <View style={styles.actions}>
            <Pressable onPress={onClose} hitSlop={8} style={styles.cancelButton}>
              <Text style={[styles.cancelText, { color: theme.textSecondary }]}>Annuler</Text>
            </Pressable>
            <Pressable
              onPress={submit}
              disabled={!value.trim()}
              style={[styles.submit, { backgroundColor: theme.accent, opacity: value.trim() ? 1 : 0.6 }]}
            >
              <Text style={styles.submitText}>{submitLabel}</Text>
            </Pressable>
          </View>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, backgroundColor: 'rgba(13, 24, 32, 0.45)', justifyContent: 'center', padding: Spacing.four },
  sheet: { ...Elevation.raised, borderRadius: Radius.sheet, padding: Spacing.four },
  title: { fontSize: 15, fontWeight: '700', marginBottom: Spacing.three },
  input: { borderWidth: 1, borderRadius: Radius.control, paddingHorizontal: Spacing.three, paddingVertical: 12, fontSize: 14 },
  actions: { flexDirection: 'row', justifyContent: 'flex-end', gap: Spacing.three, marginTop: Spacing.three },
  cancelButton: { paddingVertical: Spacing.two, paddingHorizontal: Spacing.two },
  cancelText: { fontSize: 13.5, fontWeight: '700' },
  submit: { ...Elevation.button, borderRadius: Radius.pill, paddingVertical: 12, paddingHorizontal: Spacing.four, alignItems: 'center' },
  submitText: { color: '#fff', fontWeight: '700', fontSize: 13.5 },
});
