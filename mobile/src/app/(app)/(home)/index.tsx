import { useRouter } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, View } from 'react-native';
import { Text } from '@/components/ui/text';
import { useDashboard } from '@/api/hooks/useDashboard';
import type { OccurrenceStatus } from '@/api/types';
import { ReconcileModal } from '@/components/ReconcileModal';
import Svg, { Circle } from 'react-native-svg';
import { useAuth } from '@/auth/AuthContext';
import { Aurora } from '@/components/ui/aurora';
import { WaveSparkline } from '@/components/ui/wave-sparkline';
import { Radius, Spacing } from '@/constants/theme';
import type { MonthlyOccurrence } from '@/domain/dashboard';
import { currencySymbol, formatAmount, formatMoney } from '@/domain/money';
import { useIsDesktop } from '@/hooks/use-is-desktop';
import { useScreenTopClearance } from '@/hooks/use-screen-top-clearance';
import { useTabBarClearance } from '@/hooks/use-tab-bar-clearance';
import { useTheme } from '@/hooks/use-theme';

function formatDueDate(iso: string): string {
  const [y, m, d] = iso.split('-').map(Number);
  return new Date(y, m - 1, d).toLocaleDateString('fr-FR', { day: 'numeric', month: 'short' });
}

function greeting(now = new Date()): string {
  const h = now.getHours();
  return h < 5 || h >= 18 ? 'Bonsoir' : 'Bonjour';
}

const statusLabel: Record<OccurrenceStatus, string> = {
  forecast: 'Prévu',
  reconciled: 'Rapproché',
  skipped: 'Ignoré',
};

/** Soft, stable per-account bubble colors (picked by list position). */
const ACCOUNT_TINTS = [
  ['#4fd8c4', '#23a898'],
  ['#b3a6ff', '#8b7cf0'],
  ['#8fd6f5', '#4aa9d6'],
  ['#ffc59e', '#f59a6c'],
] as const;

/** The dashboard. A soft aurora wash and a greeting up top, then a frosted
 * hero card with the total balance and its "wave" (past in lagoon, the
 * forecast in lilac). Below: accounts as rounded rows with a colored
 * bubble, this month's planned items (lilac while still ahead, green once
 * reconciled) and recent activity. Two columns on desktop (left: balance,
 * accounts, upcoming; right: recent activity), one on mobile. The full
 * interactive chart lives on its own screen (see graphs.tsx). */
export default function OverviewScreen() {
  const theme = useTheme();
  const router = useRouter();
  const { user } = useAuth();
  const isDesktop = useIsDesktop();
  const tabBarClearance = useTabBarClearance();
  const topClearance = useScreenTopClearance();
  const dashboardQuery = useDashboard();
  const data = dashboardQuery.data;
  const [reconcileTarget, setReconcileTarget] = useState<MonthlyOccurrence | null>(null);
  const firstName = user?.display_name?.split(' ')[0];
  const today = new Date().toLocaleDateString('fr-FR', { weekday: 'long', day: 'numeric', month: 'long' });

  const totalsByCurrency = new Map<string, number>();
  for (const s of data?.accountSummaries ?? []) {
    totalsByCurrency.set(s.account.currency, (totalsByCurrency.get(s.account.currency) ?? 0) + s.currentBalance);
  }

  return (
    <>
      <ScrollView
        style={[styles.container, { backgroundColor: theme.background }]}
        contentContainerStyle={[styles.content, { paddingTop: Spacing.four + topClearance, paddingBottom: Spacing.six + tabBarClearance }]}
      >
        <Aurora height={600} bleedTop={topClearance} />

        <View style={styles.greeting}>
          <Text style={[styles.greetingDate, { color: theme.textSecondary }]}>{today}</Text>
          <Text style={[styles.greetingTitle, { color: theme.text }]}>
            {greeting()}
            {firstName ? ` ${firstName}` : ''} 👋
          </Text>
        </View>

        {dashboardQuery.isLoading && <ActivityIndicator color={theme.accent} style={{ marginTop: Spacing.four }} />}
        {dashboardQuery.isError && (
          <Text style={[styles.error, { color: theme.negative }]}>Impossible de charger vos données.</Text>
        )}

        {data && (
          <View style={[styles.columns, isDesktop && styles.columnsDesktop]}>
            <View style={[styles.column, isDesktop && styles.columnLeft]}>
              {[...totalsByCurrency.entries()].map(([currency, total]) => {
                const trend = data.trends.find((t) => t.currency === currency);
                return (
                  <Pressable
                    key={currency}
                    onPress={() => router.push('/graphs')}
                    style={({ pressed }) => [
                      styles.hero,
                      {
                        backgroundColor: theme.glass,
                        borderColor: theme.glassBorder,
                        transform: [{ scale: pressed ? 0.99 : 1 }],
                      },
                    ]}
                  >
                    <View style={styles.heroTop}>
                      <Text style={[styles.heroLabel, { color: theme.textSecondary }]}>Solde total</Text>
                      <View style={[styles.chip, { backgroundColor: theme.accentPale }]}>
                        <Text style={[styles.chipText, { color: theme.accentDark }]}>Voir l&apos;évolution →</Text>
                      </View>
                    </View>
                    <Text style={[styles.heroValue, { color: total < 0 ? theme.negative : theme.text }]}>
                      {formatAmount(total, currency)}
                      <Text style={[styles.heroCurrency, { color: theme.textSecondary }]}>
                        {' '}
                        {currencySymbol(currency)}
                      </Text>
                    </Text>
                    {trend && <WaveSparkline points={trend.points} />}
                    <View style={styles.legend}>
                      <View style={[styles.legendDot, { backgroundColor: theme.accent }]} />
                      <Text style={[styles.legendText, { color: theme.textSecondary }]}>30 derniers jours</Text>
                      <View style={[styles.legendDot, { backgroundColor: theme.lilac, marginLeft: Spacing.three }]} />
                      <Text style={[styles.legendText, { color: theme.textSecondary }]}>Prévision</Text>
                    </View>
                  </Pressable>
                );
              })}

              {data.overdueRecurringCount > 0 && (
                <View style={[styles.banner, { backgroundColor: theme.forecastBg }]}>
                  <Text style={styles.bannerEmoji}>🌊</Text>
                  <Text style={{ color: theme.forecastText, fontWeight: '700', fontSize: 13.5, flex: 1 }}>
                    {data.overdueRecurringCount} élément{data.overdueRecurringCount > 1 ? 's' : ''} récurrent
                    {data.overdueRecurringCount > 1 ? 's' : ''} en attente de rapprochement
                  </Text>
                </View>
              )}

              <View>
                <Text style={[styles.sectionTitle, { color: theme.text }]}>Vos comptes</Text>
                <View style={[styles.list, { backgroundColor: theme.surface }]}>
                  {data.accountSummaries.map((s, i) => {
                    const [from, to] = ACCOUNT_TINTS[i % ACCOUNT_TINTS.length];
                    return (
                      <Pressable
                        key={s.account.id}
                        onPress={() => router.push(`/accounts/${s.account.id}`)}
                        style={({ pressed }) => [
                          styles.row,
                          { backgroundColor: pressed ? theme.surfaceSoft : 'transparent' },
                        ]}
                      >
                        <View style={[styles.bubble, { backgroundColor: from, borderColor: to }]}>
                          <Text style={styles.bubbleText}>{s.account.name.trim().charAt(0).toUpperCase()}</Text>
                        </View>
                        <Text style={[styles.rowLabel, styles.rowMain, { color: theme.text }]} numberOfLines={1}>
                          {s.account.name}
                        </Text>
                        <Text style={[styles.rowAmount, { color: s.currentBalance < 0 ? theme.negative : theme.text }]}>
                          {formatMoney(s.currentBalance, s.account.currency)}
                        </Text>
                      </Pressable>
                    );
                  })}
                  {data.accountSummaries.length === 0 && (
                    <Text style={[styles.empty, { color: theme.textSecondary }]}>
                      Aucun compte pour l&apos;instant.
                    </Text>
                  )}
                </View>
              </View>

              <View>
                <Text style={[styles.sectionTitle, { color: theme.text }]}>Échéances de ce mois</Text>
                <View style={[styles.list, { backgroundColor: theme.surface }]}>
                  {data.monthlyOccurrences.map((occ) => (
                    <View key={occ.occurrenceId} style={styles.row}>
                      <View style={[styles.dateBubble, { backgroundColor: theme.forecastBg }]}>
                        <Text style={[styles.dateBubbleText, { color: theme.forecastText }]}>
                          {formatDueDate(occ.dueDate)}
                        </Text>
                      </View>
                      <View style={styles.rowMain}>
                        <Text style={[styles.rowLabel, { color: theme.text }]} numberOfLines={1}>
                          {occ.label}
                        </Text>
                        <Text style={[styles.rowMeta, { color: theme.textSecondary }]}>
                          {formatMoney(occ.amount, occ.currency)}
                        </Text>
                      </View>
                      {occ.isBudget ? (
                        // A budget's occurrence aggregates every operation in
                        // its category automatically — there's no single
                        // operation to pick, so this shows spend-so-far
                        // instead of a clickable reconcile action.
                        <BudgetRing actual={occ.actualAmount} planned={occ.amount} />
                      ) : (
                        <Pressable
                          disabled={occ.status !== 'forecast'}
                          onPress={() => setReconcileTarget(occ)}
                          style={[
                            styles.chip,
                            { backgroundColor: occ.status === 'reconciled' ? theme.positiveBg : theme.forecastBg },
                          ]}
                        >
                          <Text
                            style={[
                              styles.chipText,
                              { color: occ.status === 'reconciled' ? theme.positive : theme.forecastText },
                            ]}
                          >
                            {occ.status === 'reconciled' ? '✓ ' : ''}
                            {statusLabel[occ.status]}
                          </Text>
                        </Pressable>
                      )}
                    </View>
                  ))}
                  {data.monthlyOccurrences.length === 0 && (
                    <Text style={[styles.empty, { color: theme.textSecondary }]}>Rien de prévu ce mois-ci ☀️</Text>
                  )}
                </View>
              </View>
            </View>

            <View style={[styles.column, isDesktop && styles.columnRight]}>
              <View>
                <Text style={[styles.sectionTitle, { color: theme.text }]}>Activité récente</Text>
                <View style={[styles.list, { backgroundColor: theme.surface }]}>
                  {data.recentActivity.map((activity, i) => {
                    const incoming = activity.amount > 0;
                    return (
                      <View key={`${activity.date}-${activity.label}-${i}`} style={styles.row}>
                        <View
                          style={[
                            styles.flowBubble,
                            { backgroundColor: incoming ? theme.positiveBg : theme.negativeBg },
                          ]}
                        >
                          <Text style={[styles.flowArrow, { color: incoming ? theme.positive : theme.negative }]}>
                            {incoming ? '↓' : '↑'}
                          </Text>
                        </View>
                        <View style={styles.rowMain}>
                          <Text style={[styles.rowLabel, { color: theme.text }]} numberOfLines={1}>
                            {activity.label}
                          </Text>
                          <Text style={[styles.rowMeta, { color: theme.textSecondary }]} numberOfLines={1}>
                            {activity.accountName}
                          </Text>
                        </View>
                        <Text style={[styles.rowAmount, { color: incoming ? theme.positive : theme.text }]}>
                          {incoming ? '+' : ''}
                          {formatMoney(activity.amount, activity.currency)}
                        </Text>
                      </View>
                    );
                  })}
                  {data.recentActivity.length === 0 && (
                    <Text style={[styles.empty, { color: theme.textSecondary }]}>Aucune activité récente.</Text>
                  )}
                </View>
              </View>
            </View>
          </View>
        )}
      </ScrollView>
      {reconcileTarget && <ReconcileModal occurrence={reconcileTarget} onClose={() => setReconcileTarget(null)} />}
    </>
  );
}

/** A small progress ring for a category budget: how much of the planned
 * amount has been spent so far. */
function BudgetRing({ actual, planned }: { actual: number | null; planned: number }) {
  const theme = useTheme();
  const size = 36;
  const stroke = 4;
  const r = (size - stroke) / 2;
  const c = 2 * Math.PI * r;
  const ratio = actual === null || planned === 0 ? 0 : Math.min(Math.abs(actual) / Math.abs(planned), 1);
  const over = actual !== null && Math.abs(actual) > Math.abs(planned);
  return (
    <View style={{ width: size, height: size, alignItems: 'center', justifyContent: 'center' }}>
      <Svg width={size} height={size} style={StyleSheet.absoluteFill}>
        <Circle cx={size / 2} cy={size / 2} r={r} stroke={theme.surfaceSoft} strokeWidth={stroke} fill="none" />
        <Circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          stroke={over ? theme.negative : theme.accent}
          strokeWidth={stroke}
          fill="none"
          strokeLinecap="round"
          strokeDasharray={`${c * ratio} ${c}`}
          transform={`rotate(-90 ${size / 2} ${size / 2})`}
        />
      </Svg>
      <Text style={{ fontSize: 9, fontWeight: '800', color: theme.textSecondary }}>{Math.round(ratio * 100)}%</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  content: { padding: Spacing.four, paddingBottom: Spacing.six },
  error: { marginTop: Spacing.three },
  greeting: { marginTop: Spacing.two, marginBottom: Spacing.four, gap: 2 },
  greetingDate: { fontSize: 13, fontWeight: '600', textTransform: 'capitalize' },
  greetingTitle: { fontSize: 30, fontWeight: '800', letterSpacing: -0.8 },
  columns: { gap: Spacing.four },
  columnsDesktop: { flexDirection: 'row', alignItems: 'flex-start', gap: Spacing.five },
  column: { gap: Spacing.four },
  // Left now carries more sections (balances, accounts, upcoming items)
  // than the right (just recent activity), so it gets more of the row.
  columnLeft: { flex: 3, minWidth: 320 },
  columnRight: { flex: 2, minWidth: 320 },
  hero: { borderRadius: Radius.sheet, borderWidth: 1, padding: Spacing.four, gap: Spacing.two },
  heroTop: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  heroLabel: { fontSize: 14, fontWeight: '600' },
  heroValue: { fontSize: 44, fontWeight: '800', letterSpacing: -1.5, fontVariant: ['tabular-nums'] },
  heroCurrency: { fontSize: 26, fontWeight: '600' },
  legend: { flexDirection: 'row', alignItems: 'center', gap: 6 },
  legendDot: { width: 8, height: 8, borderRadius: Radius.pill },
  legendText: { fontSize: 11.5, fontWeight: '600' },
  chip: { borderRadius: Radius.pill, paddingHorizontal: 12, paddingVertical: 6 },
  chipText: { fontSize: 12, fontWeight: '700' },
  banner: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: Spacing.two,
    borderRadius: Radius.card,
    padding: Spacing.three,
  },
  bannerEmoji: { fontSize: 18 },
  sectionTitle: {
    fontSize: 17,
    fontWeight: '800',
    letterSpacing: -0.3,
    marginBottom: Spacing.two,
    marginLeft: Spacing.one,
  },
  list: {
    borderRadius: Radius.card,
    padding: Spacing.one,
    shadowColor: '#1c4b5a',
    shadowOpacity: 0.06,
    shadowRadius: 18,
    shadowOffset: { width: 0, height: 6 },
  },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: 12,
    paddingVertical: 10,
    gap: 12,
    borderRadius: 18,
  },
  bubble: {
    width: 40,
    height: 40,
    borderRadius: Radius.pill,
    alignItems: 'center',
    justifyContent: 'center',
    borderBottomWidth: 3,
  },
  bubbleText: { color: '#ffffff', fontSize: 16, fontWeight: '800' },
  dateBubble: { minWidth: 52, borderRadius: 14, paddingHorizontal: 8, paddingVertical: 8, alignItems: 'center' },
  dateBubbleText: { fontSize: 12, fontWeight: '800' },
  flowBubble: { width: 40, height: 40, borderRadius: Radius.pill, alignItems: 'center', justifyContent: 'center' },
  flowArrow: { fontSize: 17, fontWeight: '800' },
  rowMain: { flex: 1, minWidth: 0 },
  rowLabel: { fontSize: 14.5, fontWeight: '700' },
  rowMeta: { fontSize: 12, fontWeight: '500', marginTop: 1 },
  rowAmount: { fontSize: 14.5, fontWeight: '700', fontVariant: ['tabular-nums'] },
  empty: { padding: Spacing.four, textAlign: 'center', fontWeight: '600' },
});
