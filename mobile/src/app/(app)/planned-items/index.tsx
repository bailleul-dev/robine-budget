import { useState } from 'react';
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, View } from 'react-native';
import { Text } from '@/components/ui/text';
import { useAccounts } from '@/api/hooks/useAccounts';
import { useCategories } from '@/api/hooks/useCategories';
import {
  useAllPlannedItems,
  useGeneratePlannedItemOccurrences,
  usePlannedItemOccurrences,
} from '@/api/hooks/usePlannedItems';
import type { Category, PlannedItem } from '@/api/types';
import { CategoryIcon } from '@/components/CategoryIcon';
import { NewBudgetSheet, type BudgetFlavor } from '@/components/NewBudgetSheet';
import { Aurora } from '@/components/ui/aurora';
import { Elevation, Radius, Spacing } from '@/constants/theme';
import { categoryDisplayName, flattenCategoryTree, rootCategoryOf } from '@/domain/category';
import { colorForSliceIndex } from '@/domain/categoryBreakdown';
import { formatMoney } from '@/domain/money';
import { useIsDesktop } from '@/hooks/use-is-desktop';
import { useScreenTopClearance } from '@/hooks/use-screen-top-clearance';
import { useTabBarClearance } from '@/hooks/use-tab-bar-clearance';
import { useTheme } from '@/hooks/use-theme';

function recurrenceLabel(
  item: Pick<PlannedItem, 'recurrence_type' | 'occurrence_count' | 'interval_weeks' | 'day_of_month'>,
): string {
  switch (item.recurrence_type) {
    case 'one_off':
      return 'Une seule fois';
    case 'perpetual':
      return item.day_of_month ? `Chaque mois, le ${item.day_of_month}` : 'Chaque mois';
    case 'fixed_count':
      return `Chaque mois pendant ${item.occurrence_count} mois`;
    case 'every_x_weeks':
      return item.interval_weeks === 1 ? 'Chaque semaine' : `Toutes les ${item.interval_weeks} semaines`;
  }
}

/** An envelope's current period (the soonest occurrence still due
 * today-or-later, falling back to the latest past one), read as "how much
 * is left" rather than as raw spent/planned numbers. actual_amount comes
 * from the backend's CategoryBudgetActuals, recomputed live on every
 * fetch. */
function EnvelopeCard({
  item,
  category,
  categories,
  tint,
  accountName,
  currency,
}: {
  item: PlannedItem;
  category: Category | undefined;
  categories: Category[];
  tint: string;
  accountName: string;
  currency: string | undefined;
}) {
  const theme = useTheme();
  const occurrencesQuery = usePlannedItemOccurrences(item.id);
  const occurrences = occurrencesQuery.data ?? [];
  const today = new Date().toISOString().slice(0, 10);
  const sorted = [...occurrences].sort((a, b) => a.due_date.localeCompare(b.due_date));
  const current = sorted.find((o) => o.due_date >= today) ?? sorted[sorted.length - 1];
  const expected = Math.abs(Number(current?.expected_amount ?? item.amount));
  const actual = current?.actual_amount != null ? Math.abs(Number(current.actual_amount)) : 0;
  const ratio = expected > 0 ? Math.min(actual / expected, 1) : 0;
  const over = actual > expected;
  const left = expected - actual;

  return (
    <View style={[styles.envelope, { backgroundColor: theme.surface }]}>
      <View style={styles.envelopeTop}>
        <View style={[styles.iconBubble, { backgroundColor: `${tint}26` }]}>
          {category ? (
            <CategoryIcon category={category} allCategories={categories} size={20} color={tint} />
          ) : (
            <Text style={{ fontSize: 18 }}>🧺</Text>
          )}
        </View>
        <View style={{ flex: 1, minWidth: 0 }}>
          <Text style={[styles.itemLabel, { color: theme.text }]} numberOfLines={1}>
            {category ? categoryDisplayName(category) : 'Enveloppe'}
          </Text>
          <Text style={[styles.itemMeta, { color: theme.textSecondary }]} numberOfLines={1}>
            {formatMoney(expected, currency)} / mois · {accountName}
          </Text>
        </View>
      </View>

      {occurrencesQuery.isLoading ? (
        <ActivityIndicator color={theme.accent} size="small" style={{ alignSelf: 'flex-start' }} />
      ) : (
        <>
          <Text style={[styles.envelopeLeft, { color: over ? theme.negative : theme.text }]}>
            {over ? `${formatMoney(-left, currency)} de trop` : `${formatMoney(left, currency)} restants`}
          </Text>
          <View style={[styles.progressTrack, { backgroundColor: theme.surfaceSoft }]}>
            <View
              style={[
                styles.progressFill,
                { width: `${Math.max(ratio, 0.02) * 100}%`, backgroundColor: over ? theme.negative : tint },
              ]}
            />
          </View>
          <Text style={[styles.itemMeta, { color: theme.subtle }]}>
            {formatMoney(actual, currency)} dépensés ce mois-ci
          </Text>
        </>
      )}
    </View>
  );
}

/** "Budgets" — a calm overview instead of a form. Two families, each with
 * its own plain-language heading: envelopes (category budgets, shown as
 * "what's left this month") and regular payments (label-matched recurring
 * charges/incomes like rent or salary). Creating either goes through
 * NewBudgetSheet, a two-step flow that also generates occurrences, so
 * there's no separate "Générer les échéances" chore. The only remaining
 * trace of it is the small "Prolonger" action, for items whose 12 months
 * of planned occurrences are running out. No delete action here (matching
 * the legacy web app). */
export default function PlannedItemsScreen() {
  const theme = useTheme();
  const isDesktop = useIsDesktop();
  const tabBarClearance = useTabBarClearance();
  const topClearance = useScreenTopClearance();
  const accountsQuery = useAccounts();
  const categoriesQuery = useCategories();
  const itemsQuery = useAllPlannedItems();
  const generateOccurrences = useGeneratePlannedItemOccurrences();

  const [sheet, setSheet] = useState<{ flavor: BudgetFlavor | null } | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [extendingId, setExtendingId] = useState<string | null>(null);

  const accounts = accountsQuery.data ?? [];
  const categories = categoriesQuery.data ?? [];
  const categoryById = new Map(categories.map((c) => [c.id, c]));
  const items = itemsQuery.data ?? [];
  const accountNameById = new Map(accounts.map((a) => [a.id, a.name]));
  const currencyByAccountId = new Map(accounts.map((a) => [a.id, a.currency]));
  // Same tint per branch as the Catégories screen (alphabetical root order).
  const rootIds = flattenCategoryTree(categories)
    .filter((e) => e.depth === 0)
    .map((e) => e.category.id);
  const tintFor = (category: Category | undefined) =>
    category ? colorForSliceIndex(Math.max(rootIds.indexOf(rootCategoryOf(category, categories).id), 0)) : theme.accent;

  const envelopes = items.filter((i) => i.match_mode === 'category');
  const regulars = items.filter((i) => i.match_mode === 'label');
  // Per currency: items on a USD account and on a EUR account can't be
  // added together. Items whose account isn't loaded yet are left out.
  const monthlyByCurrency = new Map<string, { out: number; in: number }>();
  for (const i of regulars) {
    const currency = currencyByAccountId.get(i.account_id);
    if (!currency || i.recurrence_type === 'one_off') continue;
    const totals = monthlyByCurrency.get(currency) ?? { out: 0, in: 0 };
    totals[i.kind === 'income' ? 'in' : 'out'] += Math.abs(Number(i.amount));
    monthlyByCurrency.set(currency, totals);
  }
  const monthlyTotals = [...monthlyByCurrency.entries()];

  const onExtend = async (itemId: string) => {
    setMessage(null);
    setExtendingId(itemId);
    try {
      const created = await generateOccurrences.mutateAsync({ itemId, monthsAhead: 12 });
      setMessage(
        created.length > 0
          ? `${created.length} échéance${created.length > 1 ? 's' : ''} ajoutée${created.length > 1 ? 's' : ''} ✨`
          : 'Déjà planifié pour les 12 prochains mois 👌',
      );
    } finally {
      setExtendingId(null);
    }
  };

  const openSheet = (flavor: BudgetFlavor | null) => {
    setMessage(null);
    setSheet({ flavor });
  };

  return (
    <>
      <ScrollView
        style={[styles.container, { backgroundColor: theme.background }]}
        contentContainerStyle={[styles.content, { paddingTop: Spacing.four + topClearance, paddingBottom: Spacing.six + tabBarClearance }]}
      >
        <Aurora height={420} bleedTop={topClearance} />

        <View style={[styles.hero, isDesktop && styles.heroDesktop]}>
          <View style={{ flex: 1, gap: 4 }}>
            <Text style={[styles.heroTitle, { color: theme.text }]}>Vos budgets</Text>
            <Text style={[styles.heroSubtitle, { color: theme.textSecondary }]}>
              Fixez des enveloppes et prévoyez ce qui revient chaque mois. Lagune s&apos;occupe du reste.
            </Text>
          </View>
          <Pressable
            onPress={() => openSheet(null)}
            style={({ pressed }) => [
              styles.newButton,
              Elevation.button,
              { backgroundColor: theme.accent, transform: [{ scale: pressed ? 0.97 : 1 }] },
            ]}
          >
            <Text style={[styles.newButtonText, { color: theme.onAccent }]}>＋ Nouveau budget</Text>
          </Pressable>
        </View>

        {regulars.length > 0 && (
          <View style={styles.stats}>
            <View style={[styles.stat, { backgroundColor: theme.glass, borderColor: theme.glassBorder }]}>
              <Text style={[styles.statLabel, { color: theme.textSecondary }]}>Sorties régulières</Text>
              {monthlyTotals.map(([currency, totals]) => (
                <Text key={currency} style={[styles.statValue, { color: theme.text }]}>
                  {formatMoney(totals.out, currency)}
                </Text>
              ))}
            </View>
            <View style={[styles.stat, { backgroundColor: theme.glass, borderColor: theme.glassBorder }]}>
              <Text style={[styles.statLabel, { color: theme.textSecondary }]}>Rentrées régulières</Text>
              {monthlyTotals.map(([currency, totals]) => (
                <Text key={currency} style={[styles.statValue, { color: theme.positive }]}>
                  {formatMoney(totals.in, currency)}
                </Text>
              ))}
            </View>
          </View>
        )}

        {message && (
          <View style={[styles.toast, { backgroundColor: theme.positiveBg }]}>
            <Text style={{ color: theme.positive, fontWeight: '700', fontSize: 13.5 }}>{message}</Text>
          </View>
        )}

        {itemsQuery.isLoading && <ActivityIndicator color={theme.accent} />}
        {itemsQuery.isError && <Text style={{ color: theme.negative }}>Impossible de charger les budgets.</Text>}

        {!itemsQuery.isLoading && items.length === 0 && (
          <View style={styles.emptyGrid}>
            <EmptyCard
              emoji="🧺"
              tint={theme.accentPale}
              title="Créer une enveloppe"
              description="Par exemple 400 € par mois pour les courses. Vous verrez ce qu'il reste au fil du mois."
              onPress={() => openSheet('envelope')}
            />
            <EmptyCard
              emoji="🔁"
              tint={theme.forecastBg}
              title="Prévoir un paiement régulier"
              description="Loyer, salaire, abonnements… Lagune les anticipe dans vos prévisions."
              onPress={() => openSheet('recurring')}
            />
          </View>
        )}

        <View style={[styles.columns, isDesktop && styles.columnsDesktop]}>
          {envelopes.length === 0 && regulars.length > 0 && (
            <View style={[styles.section, isDesktop && { flex: 1, paddingTop: 36 }]}>
              <HintCard
                emoji="🧺"
                title="Et si vous fixiez une enveloppe ?"
                description="Un plafond mensuel pour les courses ou les sorties, pour voir ce qu'il reste."
                onPress={() => openSheet('envelope')}
              />
            </View>
          )}
          {envelopes.length > 0 && (
            <View style={[styles.section, isDesktop && { flex: 1 }]}>
              <SectionHeader
                title="Enveloppes"
                subtitle="Un plafond par catégorie, chaque mois"
                onAdd={() => openSheet('envelope')}
              />
              <View style={styles.envelopeGrid}>
                {envelopes.map((item) => {
                  const category = item.category_id ? categoryById.get(item.category_id) : undefined;
                  return (
                    <EnvelopeCard
                      key={item.id}
                      item={item}
                      category={category}
                      categories={categories}
                      tint={tintFor(category)}
                      accountName={accountNameById.get(item.account_id) ?? '—'}
                      currency={currencyByAccountId.get(item.account_id)}
                    />
                  );
                })}
              </View>
            </View>
          )}

          {regulars.length === 0 && envelopes.length > 0 && (
            <View style={[styles.section, isDesktop && { flex: 1, paddingTop: 36 }]}>
              <HintCard
                emoji="🔁"
                title="Prévoyez vos paiements réguliers"
                description="Loyer, salaire, abonnements… ajoutés une fois, anticipés chaque mois."
                onPress={() => openSheet('recurring')}
              />
            </View>
          )}
          {regulars.length > 0 && (
            <View style={[styles.section, isDesktop && { flex: 1 }]}>
              <SectionHeader
                title="Paiements réguliers"
                subtitle="Ce qui tombe à date fixe"
                onAdd={() => openSheet('recurring')}
              />
              <View style={[styles.list, { backgroundColor: theme.surface }]}>
                {regulars.map((item) => {
                  const income = item.kind === 'income';
                  const category = item.category_id ? categoryById.get(item.category_id) : undefined;
                  return (
                    <View key={item.id} style={styles.row}>
                      <View style={[styles.flowBubble, { backgroundColor: income ? theme.positiveBg : theme.negativeBg }]}>
                        <Text style={[styles.flowArrow, { color: income ? theme.positive : theme.negative }]}>
                          {income ? '↓' : '↑'}
                        </Text>
                      </View>
                      <View style={{ flex: 1, minWidth: 0 }}>
                        <Text style={[styles.itemLabel, { color: theme.text }]} numberOfLines={1}>
                          {item.label ?? 'Paiement'}
                        </Text>
                        <Text style={[styles.itemMeta, { color: theme.textSecondary }]} numberOfLines={1}>
                          {recurrenceLabel(item)}
                          {category ? ` · ${categoryDisplayName(category)}` : ''}
                        </Text>
                      </View>
                      <View style={styles.rowRight}>
                        <Text style={[styles.itemAmount, { color: income ? theme.positive : theme.text }]}>
                          {income ? '+' : '−'}
                          {formatMoney(Math.abs(Number(item.amount)), currencyByAccountId.get(item.account_id))}
                        </Text>
                        {item.recurrence_type !== 'one_off' && (
                          <Pressable
                            onPress={() => onExtend(item.id)}
                            disabled={extendingId === item.id}
                            hitSlop={6}
                            style={[styles.extendPill, { backgroundColor: theme.surfaceSoft }]}
                          >
                            <Text style={[styles.extendText, { color: theme.textSecondary }]}>
                              {extendingId === item.id ? '…' : 'Prolonger'}
                            </Text>
                          </Pressable>
                        )}
                      </View>
                    </View>
                  );
                })}
              </View>
            </View>
          )}
        </View>
      </ScrollView>

      {sheet && (
        <NewBudgetSheet
          accounts={accounts}
          categories={categories}
          initialFlavor={sheet.flavor}
          onClose={() => setSheet(null)}
          onCreated={setMessage}
        />
      )}
    </>
  );
}

function SectionHeader({ title, subtitle, onAdd }: { title: string; subtitle: string; onAdd: () => void }) {
  const theme = useTheme();
  return (
    <View style={styles.sectionHeader}>
      <View style={{ flex: 1 }}>
        <Text style={[styles.sectionTitle, { color: theme.text }]}>{title}</Text>
        <Text style={[styles.sectionSubtitle, { color: theme.textSecondary }]}>{subtitle}</Text>
      </View>
      <Pressable onPress={onAdd} hitSlop={8} style={[styles.addPill, { backgroundColor: theme.accentPale }]}>
        <Text style={[styles.addPillText, { color: theme.accentDark }]}>＋ Ajouter</Text>
      </Pressable>
    </View>
  );
}

/** A soft, dashed nudge toward the budget family the user hasn't tried yet. */
function HintCard({
  emoji,
  title,
  description,
  onPress,
}: {
  emoji: string;
  title: string;
  description: string;
  onPress: () => void;
}) {
  const theme = useTheme();
  return (
    <Pressable
      onPress={onPress}
      style={({ pressed }) => [
        styles.hintCard,
        { borderColor: theme.lilac, backgroundColor: theme.forecastBg, opacity: pressed ? 0.8 : 1 },
      ]}
    >
      <Text style={{ fontSize: 26 }}>{emoji}</Text>
      <View style={{ flex: 1, gap: 2 }}>
        <Text style={[styles.itemLabel, { color: theme.forecastText }]}>{title}</Text>
        <Text style={[styles.sectionSubtitle, { color: theme.forecastText }]}>{description}</Text>
      </View>
      <Text style={{ fontSize: 22, color: theme.forecastText }}>›</Text>
    </Pressable>
  );
}

function EmptyCard({
  emoji,
  tint,
  title,
  description,
  onPress,
}: {
  emoji: string;
  tint: string;
  title: string;
  description: string;
  onPress: () => void;
}) {
  const theme = useTheme();
  return (
    <Pressable
      onPress={onPress}
      style={({ pressed }) => [styles.emptyCard, { backgroundColor: theme.surface, transform: [{ scale: pressed ? 0.98 : 1 }] }]}
    >
      <View style={[styles.emptyEmojiBubble, { backgroundColor: tint }]}>
        <Text style={{ fontSize: 30 }}>{emoji}</Text>
      </View>
      <Text style={[styles.itemLabel, { color: theme.text, fontSize: 16 }]}>{title}</Text>
      <Text style={[styles.sectionSubtitle, { color: theme.textSecondary }]}>{description}</Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  content: { padding: Spacing.four, gap: Spacing.four, paddingBottom: Spacing.six },
  hero: { gap: Spacing.three, marginTop: Spacing.two },
  heroDesktop: { flexDirection: 'row', alignItems: 'center' },
  heroTitle: { fontSize: 30, fontWeight: '800', letterSpacing: -0.8 },
  heroSubtitle: { fontSize: 14.5, fontWeight: '500', lineHeight: 20, maxWidth: 520 },
  newButton: { alignSelf: 'flex-start', borderRadius: Radius.pill, paddingHorizontal: Spacing.four, paddingVertical: 14 },
  newButtonText: { fontSize: 15, fontWeight: '800' },
  stats: { flexDirection: 'row', gap: Spacing.three, flexWrap: 'wrap' },
  stat: { flex: 1, minWidth: 150, borderWidth: 1, borderRadius: Radius.card, padding: Spacing.three, gap: 2 },
  statLabel: { fontSize: 12.5, fontWeight: '600' },
  statValue: { fontSize: 22, fontWeight: '800', letterSpacing: -0.5, fontVariant: ['tabular-nums'] },
  toast: { borderRadius: Radius.control, paddingHorizontal: Spacing.three, paddingVertical: 12 },
  emptyGrid: { flexDirection: 'row', flexWrap: 'wrap', gap: Spacing.three },
  emptyCard: { ...Elevation.card, flex: 1, minWidth: 240, borderRadius: Radius.card, padding: Spacing.four, gap: Spacing.two },
  emptyEmojiBubble: { width: 60, height: 60, borderRadius: 20, alignItems: 'center', justifyContent: 'center' },
  hintCard: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: Spacing.three,
    borderWidth: 1.5,
    borderStyle: 'dashed',
    borderRadius: Radius.card,
    padding: Spacing.three,
  },
  columns: { gap: Spacing.four },
  columnsDesktop: { flexDirection: 'row', alignItems: 'flex-start', gap: Spacing.five },
  section: { gap: Spacing.two },
  sectionHeader: { flexDirection: 'row', alignItems: 'center', gap: Spacing.two, paddingHorizontal: Spacing.one },
  sectionTitle: { fontSize: 17, fontWeight: '800', letterSpacing: -0.3 },
  sectionSubtitle: { fontSize: 12.5, fontWeight: '500', lineHeight: 18 },
  addPill: { borderRadius: Radius.pill, paddingHorizontal: 12, paddingVertical: 7 },
  addPillText: { fontSize: 12.5, fontWeight: '800' },
  envelopeGrid: { flexDirection: 'row', flexWrap: 'wrap', gap: Spacing.three },
  envelope: { ...Elevation.card, flex: 1, minWidth: 250, borderRadius: Radius.card, padding: Spacing.three, gap: Spacing.two },
  envelopeTop: { flexDirection: 'row', alignItems: 'center', gap: 12 },
  envelopeLeft: { fontSize: 22, fontWeight: '800', letterSpacing: -0.5, marginTop: 4, fontVariant: ['tabular-nums'] },
  iconBubble: { width: 42, height: 42, borderRadius: Radius.pill, alignItems: 'center', justifyContent: 'center' },
  list: { ...Elevation.card, borderRadius: Radius.card, padding: Spacing.one },
  row: { flexDirection: 'row', alignItems: 'center', gap: 12, paddingHorizontal: 12, paddingVertical: 10 },
  rowRight: { alignItems: 'flex-end', gap: 4 },
  flowBubble: { width: 40, height: 40, borderRadius: Radius.pill, alignItems: 'center', justifyContent: 'center' },
  flowArrow: { fontSize: 17, fontWeight: '800' },
  itemLabel: { fontSize: 15, fontWeight: '700' },
  itemMeta: { fontSize: 12.5, fontWeight: '500', marginTop: 1 },
  itemAmount: { fontSize: 15, fontWeight: '800', fontVariant: ['tabular-nums'] },
  extendPill: { borderRadius: Radius.pill, paddingHorizontal: 8, paddingVertical: 3 },
  extendText: { fontSize: 11, fontWeight: '700' },
  progressTrack: { height: 12, borderRadius: Radius.pill, overflow: 'hidden' },
  progressFill: { height: '100%', borderRadius: Radius.pill },
});
