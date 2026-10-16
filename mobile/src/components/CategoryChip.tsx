import { useState } from 'react';
import { Modal, Pressable, ScrollView, StyleSheet, type GestureResponderEvent } from 'react-native';
import { Text } from '@/components/ui/text';
import type { Category, OperationSplit } from '@/api/types';
import { CategoryIcon } from '@/components/CategoryIcon';
import { Radius, Spacing } from '@/constants/theme';
import { categoryDisplayName, flattenCategoryTree } from '@/domain/category';
import { useTheme } from '@/hooks/use-theme';

/** Small tappable tag showing an operation's current category (with its
 * icon) or "Non catégorisé" in muted style — tapping it opens a picker
 * modal to change it right there, no navigating away from the operations
 * list. When the operation is already split across several categories,
 * shows a compact summary chip instead and tapping it opens the split
 * editor directly (onOpenSplit), skipping the plain picker entirely — same
 * as the picker's own "Diviser cette opération" row when it isn't split
 * yet. Ported from frontend/src/components/CategoryPicker.tsx's
 * CategoryChip. Lives inside an operation row that's itself pressable (see
 * the account screen's "make recurring" tap) — stopPropagation keeps a tap
 * here from also opening that, same as web's onClick wrapper does. */
export function CategoryChip({
  categories,
  categoryId,
  splits = [],
  onSelect,
  onOpenSplit,
}: {
  categories: Category[];
  categoryId: string | null;
  splits?: OperationSplit[];
  onSelect: (categoryId: string | null) => void;
  onOpenSplit: () => void;
}) {
  const theme = useTheme();
  const [open, setOpen] = useState(false);
  const current = categories.find((c) => c.id === categoryId) ?? null;
  const entries = flattenCategoryTree(categories);

  const stop = (e: GestureResponderEvent) => e.stopPropagation();

  if (splits.length > 0) {
    const names = splits
      .map((s) => categories.find((c) => c.id === s.category_id))
      .filter((c): c is Category => c !== undefined)
      .map(categoryDisplayName);
    return (
      <Pressable
        onPress={(e) => {
          stop(e);
          onOpenSplit();
        }}
        style={[styles.chip, { backgroundColor: theme.surfaceSoft, borderColor: theme.line }]}
      >
        <Text style={[styles.chipText, { color: theme.text }]} numberOfLines={1}>
          {names.join(', ')}
        </Text>
      </Pressable>
    );
  }

  return (
    <>
      <Pressable
        onPress={(e) => {
          stop(e);
          setOpen(true);
        }}
        style={[styles.chip, { backgroundColor: current ? theme.surfaceSoft : 'transparent', borderColor: theme.line }]}
      >
        {current && <CategoryIcon category={current} allCategories={categories} size={13} color={theme.textSecondary} />}
        <Text style={[styles.chipText, { color: current ? theme.text : theme.subtle }]} numberOfLines={1}>
          {current ? categoryDisplayName(current) : 'Non catégorisé'}
        </Text>
      </Pressable>

      <Modal visible={open} transparent animationType="fade" onRequestClose={() => setOpen(false)}>
        <Pressable style={styles.backdrop} onPress={() => setOpen(false)}>
          <Pressable style={[styles.sheet, { backgroundColor: theme.surface, borderColor: theme.line }]}>
            <Text style={[styles.sheetTitle, { color: theme.text }]}>Catégorie</Text>
            <ScrollView style={styles.list}>
              <Pressable
                onPress={() => {
                  onSelect(null);
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
                    onSelect(category.id);
                    setOpen(false);
                  }}
                  style={[styles.row, { borderBottomColor: theme.line, paddingLeft: Spacing.four + depth * Spacing.four }]}
                >
                  {depth === 0 && (
                    <CategoryIcon
                      category={category}
                      allCategories={categories}
                      size={15}
                      color={category.id === categoryId ? theme.accentDark : theme.textSecondary}
                    />
                  )}
                  <Text style={[styles.rowText, { color: category.id === categoryId ? theme.accentDark : theme.text }]}>
                    {categoryDisplayName(category)}
                  </Text>
                </Pressable>
              ))}
            </ScrollView>
            <Pressable
              onPress={() => {
                setOpen(false);
                onOpenSplit();
              }}
              style={styles.splitRow}
            >
              <Text style={[styles.splitRowText, { color: theme.accentDark }]}>Diviser en plusieurs catégories…</Text>
            </Pressable>
          </Pressable>
        </Pressable>
      </Modal>
    </>
  );
}

const styles = StyleSheet.create({
  chip: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 4,
    alignSelf: 'flex-start',
    borderWidth: 1,
    borderRadius: 999,
    paddingHorizontal: Spacing.two,
    paddingVertical: 3,
  },
  chipText: { fontSize: 11.5, fontWeight: '600' },
  backdrop: { flex: 1, backgroundColor: 'rgba(13, 24, 32, 0.45)', justifyContent: 'flex-end' },
  sheet: { borderTopLeftRadius: Radius.sheet, borderTopRightRadius: Radius.sheet, maxHeight: '70%', padding: Spacing.four },
  sheetTitle: { fontSize: 15, fontWeight: '700', marginBottom: Spacing.two },
  list: { flexGrow: 0 },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: Spacing.two,
    paddingVertical: Spacing.three,
    paddingHorizontal: Spacing.four,
    borderBottomWidth: 1,
  },
  rowText: { fontSize: 14 },
  splitRow: { paddingVertical: Spacing.three, paddingHorizontal: Spacing.four },
  splitRowText: { fontSize: 13.5, fontWeight: '700' },
});
