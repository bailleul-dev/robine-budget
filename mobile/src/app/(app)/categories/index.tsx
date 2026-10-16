import { useState } from 'react';
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, View } from 'react-native';
import { Text } from '@/components/ui/text';
import { ApiError } from '@/api/client';
import { useCategories, useCreateCategory, useDeleteCategory, useUpdateCategory } from '@/api/hooks/useCategories';
import type { Category } from '@/api/types';
import { CategoryIcon } from '@/components/CategoryIcon';
import { PromptModal } from '@/components/PromptModal';
import { Elevation, Radius, Spacing } from '@/constants/theme';
import { categoryDisplayName, flattenCategoryTree } from '@/domain/category';
import { colorForSliceIndex } from '@/domain/categoryBreakdown';
import { useScreenTopClearance } from '@/hooks/use-screen-top-clearance';
import { useTabBarClearance } from '@/hooks/use-tab-bar-clearance';
import { useTheme } from '@/hooks/use-theme';
import { confirmAsync } from '@/lib/confirm';

type DialogState = { kind: 'add'; parent: Category } | { kind: 'rename'; category: Category } | null;

/** Manage the category tree: add a subcategory, rename or delete your own
 * ones. Standard (top-level) categories are shared and read-only — only
 * "Ajouter" shows for those. Ported from frontend/src/pages/CategoriesScreen.tsx. */
export default function CategoriesScreen() {
  const theme = useTheme();
  const tabBarClearance = useTabBarClearance();
  const topClearance = useScreenTopClearance();
  const categoriesQuery = useCategories();
  const createCategory = useCreateCategory();
  const updateCategory = useUpdateCategory();
  const deleteCategory = useDeleteCategory();
  const [dialog, setDialog] = useState<DialogState>(null);
  const [error, setError] = useState<string | null>(null);

  const categories = categoriesQuery.data ?? [];
  const entries = flattenCategoryTree(categories);
  // One card per top-level category, holding its whole subtree.
  const groups: (typeof entries)[] = [];
  for (const entry of entries) {
    if (entry.depth === 0 || groups.length === 0) groups.push([]);
    groups[groups.length - 1].push(entry);
  }

  const handleApiError = (err: unknown) => setError(err instanceof ApiError ? err.message : 'Une erreur est survenue.');

  const onDelete = async (category: Category) => {
    setError(null);
    const confirmed = await confirmAsync(
      'Supprimer la catégorie',
      `Supprimer « ${categoryDisplayName(category)} » ? Les opérations qui l'utilisent redeviendront non catégorisées.`,
      'Supprimer',
    );
    if (!confirmed) return;
    try {
      await deleteCategory.mutateAsync(category.id);
    } catch (err) {
      handleApiError(err);
    }
  };

  return (
    <ScrollView
      style={[styles.container, { backgroundColor: theme.background }]}
      contentContainerStyle={[styles.content, { paddingTop: Spacing.four + topClearance, paddingBottom: Spacing.six + tabBarClearance }]}
    >
      {categoriesQuery.isLoading && <ActivityIndicator color={theme.accent} />}
      {categoriesQuery.isError && (
        <Text style={{ color: theme.negative }}>Impossible de charger les catégories.</Text>
      )}
      {error && <Text style={{ color: theme.negative, fontSize: 12.5 }}>{error}</Text>}

      {groups.map((group, groupIndex) => {
        // Each branch gets its own soft color, reused for its icon bubble
        // and its subcategories' dots.
        const tint = colorForSliceIndex(groupIndex);
        return (
          <View key={group[0].category.id} style={[styles.card, { backgroundColor: theme.surface }]}>
            {group.map(({ category, depth }) => (
              <View
                key={category.id}
                style={[styles.row, depth > 0 && { paddingLeft: Spacing.three + depth * Spacing.four }]}
              >
                {depth === 0 ? (
                  <View style={[styles.iconBubble, { backgroundColor: `${tint}26` }]}>
                    <CategoryIcon category={category} allCategories={categories} size={18} color={tint} />
                  </View>
                ) : (
                  <View style={[styles.dot, { backgroundColor: tint }]} />
                )}
                <Text
                  style={[depth === 0 ? styles.rootName : styles.name, { color: depth === 0 ? theme.text : theme.textSecondary }]}
                  numberOfLines={1}
                >
                  {categoryDisplayName(category)}
                </Text>
                <View style={styles.actions}>
                  {(category.is_standard || category.editable) && (
                    <Pressable
                      onPress={() => setDialog({ kind: 'add', parent: category })}
                      hitSlop={6}
                      style={[styles.actionPill, { backgroundColor: theme.accentPale }]}
                    >
                      <Text style={[styles.actionText, { color: theme.accentDark }]}>+ Ajouter</Text>
                    </Pressable>
                  )}
                  {category.editable && (
                    <>
                      <Pressable
                        onPress={() => setDialog({ kind: 'rename', category })}
                        hitSlop={6}
                        style={[styles.actionPill, { backgroundColor: theme.surfaceSoft }]}
                      >
                        <Text style={[styles.actionText, { color: theme.textSecondary }]}>Renommer</Text>
                      </Pressable>
                      <Pressable
                        onPress={() => onDelete(category)}
                        hitSlop={6}
                        style={[styles.actionPill, { backgroundColor: theme.negativeBg }]}
                      >
                        <Text style={[styles.actionText, { color: theme.negative }]}>Suppr.</Text>
                      </Pressable>
                    </>
                  )}
                </View>
              </View>
            ))}
          </View>
        );
      })}

      {dialog?.kind === 'add' && (
        <PromptModal
          key={`add-${dialog.parent.id}`}
          title={`Ajouter une sous-catégorie à « ${categoryDisplayName(dialog.parent)} »`}
          submitLabel="Ajouter"
          onClose={() => setDialog(null)}
          onSubmit={async (name) => {
            try {
              await createCategory.mutateAsync({ name, parentId: dialog.parent.id });
              setDialog(null);
            } catch (err) {
              handleApiError(err);
            }
          }}
        />
      )}
      {dialog?.kind === 'rename' && (
        <PromptModal
          key={`rename-${dialog.category.id}`}
          title="Renommer la catégorie"
          initialValue={dialog.category.name}
          onClose={() => setDialog(null)}
          onSubmit={async (name) => {
            if (name === dialog.category.name || dialog.category.parent_id === null) {
              setDialog(null);
              return;
            }
            try {
              await updateCategory.mutateAsync({ id: dialog.category.id, name, parentId: dialog.category.parent_id });
              setDialog(null);
            } catch (err) {
              handleApiError(err);
            }
          }}
        />
      )}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  content: { padding: Spacing.four, gap: Spacing.three, paddingBottom: Spacing.six },
  card: { ...Elevation.card, borderRadius: Radius.card, paddingVertical: Spacing.two },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
    paddingVertical: Spacing.two,
    paddingLeft: Spacing.three,
    paddingRight: Spacing.three,
  },
  iconBubble: { width: 38, height: 38, borderRadius: Radius.pill, alignItems: 'center', justifyContent: 'center' },
  dot: { width: 8, height: 8, borderRadius: Radius.pill },
  rootName: { fontSize: 15.5, fontWeight: '800', letterSpacing: -0.2, flex: 1, minWidth: 0 },
  name: { fontSize: 14, fontWeight: '600', flex: 1, minWidth: 0 },
  actions: { flexDirection: 'row', gap: 6 },
  actionPill: { borderRadius: Radius.pill, paddingHorizontal: 10, paddingVertical: 5 },
  actionText: { fontSize: 11.5, fontWeight: '700' },
});
