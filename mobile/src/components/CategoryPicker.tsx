import { useState } from 'react';
import { Modal, Pressable, ScrollView, StyleSheet } from 'react-native';
import { Text } from '@/components/ui/text';
import type { Category } from '@/api/types';
import { categoryDisplayName, flattenCategoryTree } from '@/domain/category';
import { Radius, Spacing } from '@/constants/theme';
import { useTheme } from '@/hooks/use-theme';

/** No native <select> in RN — a Pressable "field" that opens a simple modal
 * list (root-first, indented by depth via flattenCategoryTree) instead.
 * Used by the recurring-item form for now; general enough to reuse once
 * operations get their own category picker. */
export function CategoryPicker({
  categories,
  value,
  onChange,
  placeholder = 'Non catégorisé',
}: {
  categories: Category[];
  value: string | null;
  onChange: (categoryId: string | null) => void;
  /** Shown when nothing is selected. */
  placeholder?: string;
}) {
  const theme = useTheme();
  const [open, setOpen] = useState(false);
  const entries = flattenCategoryTree(categories);
  const selected = categories.find((c) => c.id === value);

  return (
    <>
      <Pressable
        onPress={() => setOpen(true)}
        style={[styles.field, { borderColor: theme.line, backgroundColor: theme.surface }]}
      >
        <Text style={[styles.fieldText, { color: selected ? theme.text : theme.subtle }]} numberOfLines={1}>
          {selected ? categoryDisplayName(selected) : placeholder}
        </Text>
      </Pressable>

      <Modal visible={open} transparent animationType="fade" onRequestClose={() => setOpen(false)}>
        <Pressable style={styles.backdrop} onPress={() => setOpen(false)}>
          <Pressable style={[styles.sheet, { backgroundColor: theme.surface, borderColor: theme.line }]}>
            <Text style={[styles.sheetTitle, { color: theme.text }]}>Catégorie</Text>
            <ScrollView style={styles.list}>
              <Pressable
                onPress={() => {
                  onChange(null);
                  setOpen(false);
                }}
                style={[styles.row, { borderBottomColor: theme.line }]}
              >
                <Text style={[styles.rowText, { color: theme.textSecondary }]}>Non catégorisé</Text>
              </Pressable>
              {entries.map(({ category, depth }) => (
                <Pressable
                  key={category.id}
                  onPress={() => {
                    onChange(category.id);
                    setOpen(false);
                  }}
                  style={[styles.row, { borderBottomColor: theme.line, paddingLeft: Spacing.four + depth * Spacing.four }]}
                >
                  <Text style={[styles.rowText, { color: category.id === value ? theme.accentDark : theme.text }]}>
                    {categoryDisplayName(category)}
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
