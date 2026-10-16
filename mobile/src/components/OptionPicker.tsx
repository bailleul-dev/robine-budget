import { useState } from 'react';
import { Modal, Pressable, ScrollView, StyleSheet } from 'react-native';
import { Text } from '@/components/ui/text';
import { Radius, Spacing } from '@/constants/theme';
import { useTheme } from '@/hooks/use-theme';

export type PickerOption<T extends string> = { value: T; label: string };

/** Same modal-list pattern as CategoryPicker, without the tree/indentation
 * bit — a flat list of value/label pairs. Used for account, kind and
 * recurrence-type fields in the recurring-item form (no native <select> in
 * RN to fall back on). */
export function OptionPicker<T extends string>({
  value,
  options,
  onChange,
  placeholder,
  title,
}: {
  value: T | null;
  options: PickerOption<T>[];
  onChange: (v: T) => void;
  placeholder: string;
  title: string;
}) {
  const theme = useTheme();
  const [open, setOpen] = useState(false);
  const selected = options.find((o) => o.value === value);

  return (
    <>
      <Pressable
        onPress={() => setOpen(true)}
        style={[styles.field, { borderColor: theme.line, backgroundColor: theme.surface }]}
      >
        <Text style={[styles.fieldText, { color: selected ? theme.text : theme.subtle }]} numberOfLines={1}>
          {selected?.label ?? placeholder}
        </Text>
      </Pressable>

      <Modal visible={open} transparent animationType="fade" onRequestClose={() => setOpen(false)}>
        <Pressable style={styles.backdrop} onPress={() => setOpen(false)}>
          <Pressable style={[styles.sheet, { backgroundColor: theme.surface, borderColor: theme.line }]}>
            <Text style={[styles.sheetTitle, { color: theme.text }]}>{title}</Text>
            <ScrollView style={styles.list}>
              {options.map((option) => (
                <Pressable
                  key={option.value}
                  onPress={() => {
                    onChange(option.value);
                    setOpen(false);
                  }}
                  style={[styles.row, { borderBottomColor: theme.line }]}
                >
                  <Text style={[styles.rowText, { color: option.value === value ? theme.accentDark : theme.text }]}>
                    {option.label}
                  </Text>
                </Pressable>
              ))}
            </ScrollView>
          </Pressable>
        </Pressable>
      </Modal>
    </>
  );
}

const styles = StyleSheet.create({
  field: { borderWidth: 1, borderRadius: Radius.control, paddingHorizontal: Spacing.three, paddingVertical: 12 },
  fieldText: { fontSize: 14 },
  backdrop: { flex: 1, backgroundColor: 'rgba(13, 24, 32, 0.45)', justifyContent: 'flex-end' },
  sheet: { borderTopLeftRadius: Radius.sheet, borderTopRightRadius: Radius.sheet, maxHeight: '70%', padding: Spacing.four },
  sheetTitle: { fontSize: 15, fontWeight: '700', marginBottom: Spacing.two },
  list: { flexGrow: 0 },
  row: { paddingVertical: Spacing.three, paddingHorizontal: Spacing.four, borderBottomWidth: 1 },
  rowText: { fontSize: 14 },
});
