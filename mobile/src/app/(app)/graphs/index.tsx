import { useRef, useState } from 'react';
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, View, type GestureResponderEvent } from 'react-native';
import { Text } from '@/components/ui/text';
import Svg, { Circle, Defs, G, Line, LinearGradient, Path, Stop, Text as SvgText } from 'react-native-svg';
import { useCategoryBreakdown } from '@/api/hooks/useCategoryBreakdown';
import { PeriodPicker } from '@/components/PeriodPicker';
import { useDashboard } from '@/api/hooks/useDashboard';
import { colorForSliceIndex, type CurrencyBreakdown } from '@/domain/categoryBreakdown';
import type { BalancePoint, CurrencyTrend, DailyOperation } from '@/domain/dashboard';
import { formatAmount, formatMoney } from '@/domain/money';
import { periodRange, type Period, type PeriodRange } from '@/domain/period';
import { Elevation, Radius, Spacing, Typefaces } from '@/constants/theme';
import { useScreenTopClearance } from '@/hooks/use-screen-top-clearance';
import { useTabBarClearance } from '@/hooks/use-tab-bar-clearance';
import { useTheme } from '@/hooks/use-theme';

const CHART_WIDTH = 340;
const CHART_HEIGHT = 220;
const PADDING_Y = 16;
const PADDING_X = 4;

const PIE_SIZE = 260;
const PIE_RADIUS = 95;
const PIE_STROKE = 32;
const PIE_GAP = 1.5;

type GraphTab = 'balance' | 'categories';

function formatShortDate(iso: string): string {
  const [, m, d] = iso.split('-');
  const months = ['janv.', 'févr.', 'mars', 'avr.', 'mai', 'juin', 'juil.', 'août', 'sept.', 'oct.', 'nov.', 'déc.'];
  return `${Number(d)} ${months[Number(m) - 1]}`;
}

/** Builds an SVG path string for a run of points, scaled into the chart's
 * pixel box. `points` and `all` share the same index space (`all` is the
 * full series) so x positions line up between the "actual" and "forecast"
 * segments drawn as separate paths, and between every account's own line
 * and the total. `width` is the chart's actual on-screen width (the
 * measured container, not the CHART_WIDTH fallback) so the plotted line
 * fills whatever space is available instead of a fixed 340px box centered
 * (and mostly empty) inside a wider card. */
function pathFor(points: BalancePoint[], startIndex: number, all: BalancePoint[], min: number, max: number, width: number): string {
  const xScale = (i: number) => PADDING_X + (i / (all.length - 1)) * (width - 2 * PADDING_X);
  const yScale = (v: number) => {
    if (max === min) return CHART_HEIGHT / 2;
    const t = (v - min) / (max - min);
    return CHART_HEIGHT - PADDING_Y - t * (CHART_HEIGHT - 2 * PADDING_Y);
  };
  return points
    .map((p, i) => `${i === 0 ? 'M' : 'L'} ${xScale(startIndex + i).toFixed(1)} ${yScale(p.balance).toFixed(1)}`)
    .join(' ');
}

function splitPastForecast(points: BalancePoint[]): { past: BalancePoint[]; forecast: BalancePoint[]; splitIndex: number } {
  const splitIndex = points.findIndex((p) => p.isForecast);
  const past = splitIndex === -1 ? points : points.slice(0, splitIndex + 1);
  const forecast = splitIndex === -1 ? [] : points.slice(splitIndex);
  return { past, forecast, splitIndex };
}

/** Line chart: the total per currency (thick, accent-colored) plus one
 * thinner line per account (see trend.accountTrends — already computed in
 * domain/dashboard.ts, just not drawn before), a value scale (min/mid/max
 * gridlines), and a press-and-drag scrubber — hold and slide a finger or
 * the mouse along the chart to see the exact date, every account's balance
 * that day, and any real operations that happened on it (from
 * operationsByDate; forecast days never have any). */
function CurrencyChart({
  trend,
  operationsByDate,
}: {
  trend: CurrencyTrend;
  operationsByDate: Record<string, DailyOperation[]>;
}) {
  const theme = useTheme();
  const points = trend.points;
  const { past: pastPoints, forecast: forecastPoints, splitIndex } = splitPastForecast(points);

  const [containerWidth, setContainerWidth] = useState(0);
  const [scrubIndex, setScrubIndex] = useState<number | null>(null);
  const chartWidth = containerWidth || CHART_WIDTH;
  const chartAreaRef = useRef<View>(null);

  const values = points.map((p) => p.balance);
  const rawMin = Math.min(...values);
  const rawMax = Math.max(...values);
  // Anchored at 0 (not the data's own min) so the line's height on screen
  // reflects the real balance relative to zero, instead of exaggerating
  // small fluctuations into dramatic-looking swings. Still extends below 0
  // when the balance actually goes negative (overdraft), rather than
  // clipping it.
  const min = Math.min(0, rawMin);
  const max = rawMax === min ? min + 1 : rawMax;
  const mid = (min + max) / 2;

  const yScale = (v: number) => {
    if (max === min) return CHART_HEIGHT / 2;
    const t = (v - min) / (max - min);
    return CHART_HEIGHT - PADDING_Y - t * (CHART_HEIGHT - 2 * PADDING_Y);
  };
  const xScale = (i: number) => PADDING_X + (i / (points.length - 1)) * (chartWidth - 2 * PADDING_X);

  const pastPath = pathFor(pastPoints, 0, points, min, max, chartWidth);
  const forecastPath = forecastPoints.length > 0 ? pathFor(forecastPoints, splitIndex, points, min, max, chartWidth) : '';
  const todayX = splitIndex === -1 ? chartWidth - PADDING_X : PADDING_X + (splitIndex / (points.length - 1)) * (chartWidth - 2 * PADDING_X);
  const lastForecast = forecastPoints.at(-1);

  const accountLines = trend.accountTrends.map((account, i) => {
    const { past, forecast, splitIndex: accSplit } = splitPastForecast(account.points);
    return {
      account,
      color: colorForSliceIndex(i + 1),
      pastPath: pathFor(past, 0, account.points, min, max, chartWidth),
      forecastPath: forecast.length > 0 ? pathFor(forecast, accSplit, account.points, min, max, chartWidth) : '',
    };
  });

  const handleScrub = (e: GestureResponderEvent) => {
    if (containerWidth <= 0) return;
    const fraction = Math.min(Math.max(e.nativeEvent.locationX / containerWidth, 0), 1);
    setScrubIndex(Math.round(fraction * (points.length - 1)));
  };
  const clearScrub = () => setScrubIndex(null);

  // Web-only: react-native-web's touch-responder emulation (onResponderGrant/
  // Move above) only tracks the pointer while a button is held down, so a
  // plain mouse hover never fires it — this is what actually drives the
  // tooltip from a hover, no click/drag needed. Native ignores these props
  // harmlessly (RN has no onMouseMove). clientX is relative to the viewport,
  // not this element, hence the ref + getBoundingClientRect to find the
  // chart's own left edge.
  const handleMouseMove = (e: { nativeEvent: { clientX: number } }) => {
    if (containerWidth <= 0) return;
    const node = chartAreaRef.current as unknown as HTMLElement | null;
    const rect = node?.getBoundingClientRect?.();
    if (!rect) return;
    const fraction = Math.min(Math.max((e.nativeEvent.clientX - rect.left) / containerWidth, 0), 1);
    setScrubIndex(Math.round(fraction * (points.length - 1)));
  };

  const scrubPoint = scrubIndex !== null ? points[scrubIndex] : null;
  const scrubDayOperations = scrubPoint ? (operationsByDate[scrubPoint.date] ?? []) : [];
  const scrubViewBoxX = scrubIndex !== null ? xScale(scrubIndex) : 0;
  const scrubPixelFraction = scrubIndex !== null ? scrubIndex / (points.length - 1) : 0;
  const tooltipOnRight = scrubPixelFraction < 0.5;

  return (
    <View style={[styles.card, { backgroundColor: theme.surface, borderColor: theme.line }]}>
      <View style={styles.cardHeader}>
        <Text style={[styles.cardTitle, { color: theme.text }]}>Évolution du solde · {trend.currency}</Text>
        <View style={styles.legend}>
          <View style={styles.legendItem}>
            <View style={[styles.legendLine, { backgroundColor: theme.accent }]} />
            <Text style={[styles.legendText, { color: theme.textSecondary }]}>Total</Text>
          </View>
          {accountLines.map(({ account, color }) => (
            <View key={account.accountId} style={styles.legendItem}>
              <View style={[styles.legendLine, { backgroundColor: color }]} />
              <Text style={[styles.legendText, { color: theme.textSecondary }]} numberOfLines={1}>
                {account.accountName}
              </Text>
            </View>
          ))}
        </View>
      </View>

      <View
        ref={chartAreaRef}
        style={styles.chartArea}
        onLayout={(e) => setContainerWidth(e.nativeEvent.layout.width)}
        onStartShouldSetResponder={() => true}
        onMoveShouldSetResponder={() => true}
        onResponderGrant={handleScrub}
        onResponderMove={handleScrub}
        onResponderRelease={clearScrub}
        onResponderTerminate={clearScrub}
        // @ts-expect-error web-only DOM event, see handleMouseMove above
        onMouseMove={handleMouseMove}
        onMouseLeave={clearScrub}
      >
        <Svg width={chartWidth} height={CHART_HEIGHT} viewBox={`0 0 ${chartWidth} ${CHART_HEIGHT}`}>
          <Defs>
            <LinearGradient id="total-fill" x1="0" y1="0" x2="0" y2="1">
              <Stop offset="0" stopColor={theme.accent} stopOpacity={0.22} />
              <Stop offset="1" stopColor={theme.accent} stopOpacity={0} />
            </LinearGradient>
          </Defs>
          {[min, mid, max].map((v, i) => (
            <G key={i}>
              <Line
                x1={PADDING_X}
                y1={yScale(v)}
                x2={chartWidth - PADDING_X}
                y2={yScale(v)}
                stroke={theme.line}
                strokeWidth={1}
                strokeDasharray="2,4"
              />
              <SvgText x={PADDING_X} y={yScale(v) - 3} fontSize={9} fill={theme.subtle}>
                {formatAmount(v, trend.currency)}
              </SvgText>
            </G>
          ))}
          <Line x1={todayX} y1={0} x2={todayX} y2={CHART_HEIGHT} stroke={theme.line} strokeWidth={1} strokeDasharray="3,3" />

          {accountLines.map(({ account, color, pastPath: accPast, forecastPath: accForecast }) => (
            <G key={account.accountId}>
              {accForecast !== '' && (
                <Path d={accForecast} stroke={color} strokeWidth={1.5} fill="none" strokeDasharray="1,5" strokeLinecap="round" strokeLinejoin="round" opacity={0.7} />
              )}
              <Path d={accPast} stroke={color} strokeWidth={2} fill="none" strokeLinecap="round" strokeLinejoin="round" opacity={0.85} />
            </G>
          ))}

          {forecastPath !== '' && (
            <Path d={forecastPath} stroke={theme.lilac} strokeWidth={3} fill="none" strokeDasharray="1,7" strokeLinecap="round" />
          )}
          {/* Soft lagoon wash under the actual total, down to the chart's floor. */}
          {pastPath !== '' && (
            <Path d={`${pastPath} L ${todayX} ${CHART_HEIGHT} L ${PADDING_X} ${CHART_HEIGHT} Z`} fill="url(#total-fill)" />
          )}
          <Path d={pastPath} stroke={theme.accent} strokeWidth={3} fill="none" strokeLinecap="round" strokeLinejoin="round" />
          <Circle cx={todayX} cy={yScale(pastPoints.at(-1)?.balance ?? 0)} r={6} fill={theme.surface} />
          <Circle cx={todayX} cy={yScale(pastPoints.at(-1)?.balance ?? 0)} r={4} fill={theme.accent} />

          {scrubPoint && (
            <G>
              <Line x1={scrubViewBoxX} y1={0} x2={scrubViewBoxX} y2={CHART_HEIGHT} stroke={theme.text} strokeWidth={1} opacity={0.4} />
              <Circle cx={scrubViewBoxX} cy={yScale(scrubPoint.balance)} r={4} fill={theme.accent} stroke={theme.surface} strokeWidth={1.5} />
              {accountLines.map(({ account, color }) => (
                <Circle
                  key={account.accountId}
                  cx={scrubViewBoxX}
                  cy={yScale(account.points[scrubIndex!].balance)}
                  r={3}
                  fill={color}
                  stroke={theme.surface}
                  strokeWidth={1}
                />
              ))}
            </G>
          )}
        </Svg>

        {scrubPoint && (
          <View
            pointerEvents="none"
            style={[
              styles.tooltip,
              { backgroundColor: theme.surface, borderColor: theme.line },
              tooltipOnRight ? { left: 4 } : { right: 4 },
            ]}
          >
            <Text style={[styles.tooltipDate, { color: theme.text }]}>
              {formatShortDate(scrubPoint.date)}
              {scrubPoint.isForecast ? ' (prévu)' : ''}
            </Text>
            <Text style={[styles.tooltipTotal, { color: theme.text }]}>
              Total : {formatMoney(scrubPoint.balance, trend.currency)}
            </Text>
            {accountLines.map(({ account, color }) => (
              <View key={account.accountId} style={styles.tooltipAccountRow}>
                <View style={[styles.legendLine, { backgroundColor: color }]} />
                <Text style={[styles.tooltipAccountText, { color: theme.textSecondary }]} numberOfLines={1}>
                  {account.accountName} : {formatAmount(account.points[scrubIndex!].balance, trend.currency)}
                </Text>
              </View>
            ))}
            {scrubDayOperations.length > 0 && (
              <View style={[styles.tooltipOps, { borderTopColor: theme.line }]}>
                {scrubDayOperations.slice(0, 4).map((op, i) => (
                  <Text key={i} style={[styles.tooltipOpText, { color: op.amount < 0 ? theme.negative : theme.positive }]} numberOfLines={1}>
                    {op.label} : {formatAmount(op.amount, trend.currency)}
                  </Text>
                ))}
              </View>
            )}
          </View>
        )}
      </View>

      <View style={styles.axisRow}>
        <Text style={[styles.axisLabel, { color: theme.subtle }]}>{formatShortDate(points[0].date)}</Text>
        <Text style={[styles.axisLabel, { color: theme.subtle }]}>Aujourd&apos;hui</Text>
        <Text style={[styles.axisLabel, { color: theme.subtle }]}>{formatShortDate(points.at(-1)!.date)}</Text>
      </View>
      <Text style={[styles.scrubHint, { color: theme.subtle }]}>Survolez (ou maintenez et glissez) le graphique pour explorer.</Text>

      <View style={styles.totalsRow}>
        <Text style={[styles.currentTotal, { color: trend.currentTotal < 0 ? theme.negative : theme.text }]}>
          Solde actuel : {formatMoney(trend.currentTotal, trend.currency)}
        </Text>
        {lastForecast && (
          <Text style={[styles.forecastTotal, { color: theme.textSecondary }]}>
            Solde prévu au {formatShortDate(lastForecast.date)} : {formatMoney(lastForecast.balance, trend.currency)}
          </Text>
        )}
      </View>
    </View>
  );
}

/** Which slice (index into `fractions`) the point (x, y) of the donut's own
 * box falls on, or null off the ring. Slices are dashes along one circle,
 * starting at 12 o'clock and running clockwise (see the -90° rotation
 * below), so the pointer's angle alone says which one it's on. */
function sliceAt(x: number, y: number, fractions: number[]): number | null {
  const dx = x - PIE_SIZE / 2;
  const dy = y - PIE_SIZE / 2;
  const distance = Math.hypot(dx, dy);
  // A little slack either side of the stroke so thin slices stay easy to hit.
  if (distance < PIE_RADIUS - PIE_STROKE / 2 - 8 || distance > PIE_RADIUS + PIE_STROKE / 2 + 8) return null;
  const fraction = ((Math.atan2(dy, dx) + Math.PI / 2) / (2 * Math.PI) + 1) % 1;
  let cumulative = 0;
  for (let i = 0; i < fractions.length; i++) {
    cumulative += fractions[i];
    if (fraction < cumulative) return i;
  }
  return null;
}

function truncate(text: string, max: number): string {
  return text.length > max ? `${text.slice(0, max - 1)}…` : text;
}

/** Donut chart built from stacked stroked circles (each slice is a dashed
 * arc of its own color) rather than SVG arc <Path> wedges — simpler math,
 * and there's no wedge-at-0%-or-100% edge case to special-case. Grouped by
 * top-level category (see domain/categoryBreakdown.ts) so a handful of
 * custom subcategories don't turn the legend into a wall of slivers.
 *
 * Hovering a slice (web) or touching it (native) — or its legend row —
 * brings it forward: thicker, the others faded, and its name, amount and
 * share in the donut's center instead of the total. */
function CategoryPieChart({ breakdown, range }: { breakdown: CurrencyBreakdown; range: PeriodRange }) {
  const theme = useTheme();
  const [active, setActive] = useState<number | null>(null);
  const donutRef = useRef<View>(null);
  const cx = PIE_SIZE / 2;
  const cy = PIE_SIZE / 2;
  const circumference = 2 * Math.PI * PIE_RADIUS;

  const fractions = breakdown.slices.map((slice) => (breakdown.total > 0 ? slice.amount / breakdown.total : 0));
  const arcs = breakdown.slices.map((slice, i) => {
    const cumulativeBefore = fractions.slice(0, i).reduce((sum, f) => sum + f, 0);
    const dashLen = Math.max(fractions[i] * circumference - PIE_GAP, 0);
    return { slice, color: colorForSliceIndex(i), dashLen, offset: -(cumulativeBefore * circumference) };
  });
  const activeArc = active !== null ? arcs[active] : undefined;

  const handleTouch = (e: GestureResponderEvent) => setActive(sliceAt(e.nativeEvent.locationX, e.nativeEvent.locationY, fractions));
  // Web-only hover, same approach as CurrencyChart's handleMouseMove.
  const handleMouseMove = (e: { nativeEvent: { clientX: number; clientY: number } }) => {
    const rect = (donutRef.current as unknown as HTMLElement | null)?.getBoundingClientRect?.();
    if (!rect) return;
    setActive(sliceAt(e.nativeEvent.clientX - rect.left, e.nativeEvent.clientY - rect.top, fractions));
  };

  return (
    <View style={[styles.card, { backgroundColor: theme.surface, borderColor: theme.line }]}>
      <Text style={[styles.cardTitle, { color: theme.text }]}>Dépenses par catégorie · {breakdown.currency}</Text>
      {/* The period itself is right above, in the picker — only what it
          adds up to per month is worth repeating here. */}
      <Text style={[styles.pieSubtitle, { color: theme.textSecondary }]}>
        {range.months > 1 && breakdown.total > 0
          ? `≈ ${formatMoney(breakdown.total / range.months, breakdown.currency)} par mois en moyenne`
          : ' '}
      </Text>

      {breakdown.slices.length === 0 ? (
        <Text style={{ color: theme.textSecondary, marginTop: Spacing.four }}>Aucune dépense sur cette période.</Text>
      ) : (
        <View style={styles.pieRow}>
          <View
            ref={donutRef}
            style={styles.donut}
            onStartShouldSetResponder={() => true}
            onMoveShouldSetResponder={() => true}
            onResponderGrant={handleTouch}
            onResponderMove={handleTouch}
            // @ts-expect-error web-only DOM events, see handleMouseMove above
            onMouseMove={handleMouseMove}
            onMouseLeave={() => setActive(null)}
          >
            <Svg width={PIE_SIZE} height={PIE_SIZE} viewBox={`0 0 ${PIE_SIZE} ${PIE_SIZE}`}>
              <G transform={`rotate(-90 ${cx} ${cy})`}>
                <Circle cx={cx} cy={cy} r={PIE_RADIUS} stroke={theme.line} strokeWidth={PIE_STROKE} fill="none" />
                {arcs.map(({ slice, color, dashLen, offset }, i) => (
                  <Circle
                    key={slice.categoryId}
                    cx={cx}
                    cy={cy}
                    r={PIE_RADIUS}
                    stroke={color}
                    strokeWidth={i === active ? PIE_STROKE + 8 : PIE_STROKE}
                    opacity={active === null || i === active ? 1 : 0.3}
                    fill="none"
                    strokeDasharray={`${dashLen} ${circumference - dashLen}`}
                    strokeDashoffset={offset}
                    strokeLinecap="butt"
                  />
                ))}
              </G>
              {activeArc ? (
                <>
                  <SvgText x={cx} y={cy - 16} fontSize={12} fontFamily={Typefaces[700]} fill={activeArc.color} textAnchor="middle">
                    {truncate(activeArc.slice.name, 18)}
                  </SvgText>
                  <SvgText x={cx} y={cy + 4} fontSize={15} fontFamily={Typefaces[800]} fill={theme.text} textAnchor="middle">
                    {formatAmount(activeArc.slice.amount, breakdown.currency)}
                  </SvgText>
                  <SvgText x={cx} y={cy + 21} fontSize={10} fontFamily={Typefaces[600]} fill={theme.textSecondary} textAnchor="middle">
                    {`${Math.round(fractions[active!] * 100)} % · ${breakdown.currency}`}
                  </SvgText>
                </>
              ) : (
                <>
                  <SvgText x={cx} y={cy - 4} fontSize={15} fontFamily={Typefaces[800]} fill={theme.text} textAnchor="middle">
                    {formatAmount(breakdown.total, breakdown.currency)}
                  </SvgText>
                  <SvgText x={cx} y={cy + 13} fontSize={10} fontFamily={Typefaces[600]} fill={theme.textSecondary} textAnchor="middle">
                    {breakdown.currency}
                  </SvgText>
                </>
              )}
            </Svg>
          </View>

          <View style={styles.pieLegend}>
            {arcs.map(({ slice, color }, i) => {
              const pct = breakdown.total > 0 ? Math.round((slice.amount / breakdown.total) * 100) : 0;
              return (
                <Pressable
                  key={slice.categoryId}
                  onHoverIn={() => setActive(i)}
                  onHoverOut={() => setActive(null)}
                  onPress={() => setActive((current) => (current === i ? null : i))}
                  style={[
                    styles.pieLegendRow,
                    i === active && { backgroundColor: theme.surfaceSoft },
                    active !== null && i !== active && { opacity: 0.5 },
                  ]}
                >
                  <View style={[styles.pieLegendDot, { backgroundColor: color }]} />
                  <Text style={[styles.pieLegendName, { color: theme.text }]} numberOfLines={1}>
                    {slice.name}
                  </Text>
                  <Text style={[styles.pieLegendAmount, { color: theme.textSecondary }]}>
                    {formatAmount(slice.amount, breakdown.currency)}&nbsp;· {pct}%
                  </Text>
                </Pressable>
              );
            })}
          </View>
        </View>
      )}
    </View>
  );
}

/** The one piece of the web dashboard that hadn't been ported yet — the
 * balance trend chart — now drawn with react-native-svg instead of
 * recharts (web-only). Reuses useDashboard's already-fetched trend data,
 * so opening this screen costs no extra request when coming from Aperçu.
 * The category pie chart is mobile-only for now (no web equivalent exists
 * yet) — its own fetch of this month's operations. A tab switcher shows
 * one chart at a time, each one full width (no more side-by-side cap that
 * left both cramped on a wide screen). */
export default function GraphsScreen() {
  const theme = useTheme();
  const tabBarClearance = useTabBarClearance();
  const topClearance = useScreenTopClearance();
  const dashboardQuery = useDashboard();
  const data = dashboardQuery.data;
  const [period, setPeriod] = useState<Period>({ preset: 'month', offset: 0 });
  const range = periodRange(period);
  const breakdownQuery = useCategoryBreakdown(range);
  const breakdowns = breakdownQuery.data ?? [];
  const [tab, setTab] = useState<GraphTab>('balance');

  return (
    <ScrollView
      style={[styles.container, { backgroundColor: theme.background }]}
      contentContainerStyle={[styles.content, { paddingTop: Spacing.four + topClearance, paddingBottom: Spacing.six + tabBarClearance }]}
    >
      <View style={[styles.tabs, { backgroundColor: theme.surfaceSoft }]}>
        <Pressable
          onPress={() => setTab('balance')}
          style={[styles.tab, tab === 'balance' && { backgroundColor: theme.surface }]}
        >
          <Text style={[styles.tabText, { color: tab === 'balance' ? theme.accentDark : theme.textSecondary }]}>
            Évolution du solde
          </Text>
        </Pressable>
        <Pressable
          onPress={() => setTab('categories')}
          style={[styles.tab, tab === 'categories' && { backgroundColor: theme.surface }]}
        >
          <Text style={[styles.tabText, { color: tab === 'categories' ? theme.accentDark : theme.textSecondary }]}>
            Répartition par catégorie
          </Text>
        </Pressable>
      </View>

      {tab === 'balance' ? (
        <>
          {dashboardQuery.isLoading && <ActivityIndicator color={theme.accent} />}
          {dashboardQuery.isError && (
            <Text style={{ color: theme.negative }}>Impossible de charger les données.</Text>
          )}
          <View style={styles.grid}>
            {data?.trends.map((trend) => (
              <CurrencyChart key={trend.currency} trend={trend} operationsByDate={data.operationsByDate} />
            ))}
          </View>
          {data && data.trends.length === 0 && (
            <Text style={{ color: theme.textSecondary }}>Aucune donnée à afficher pour l&apos;instant.</Text>
          )}
        </>
      ) : (
        <>
          <PeriodPicker value={period} onChange={setPeriod} />
          {breakdownQuery.isLoading && <ActivityIndicator color={theme.accent} />}
          {breakdownQuery.isError && (
            <Text style={{ color: theme.negative }}>Impossible de charger la répartition par catégorie.</Text>
          )}
          {/* While another period loads, the previous one stays up, dimmed. */}
          <View style={[styles.grid, breakdownQuery.isPlaceholderData && styles.stale]}>
            {breakdowns.map((breakdown) => (
              <CategoryPieChart key={breakdown.currency} breakdown={breakdown} range={range} />
            ))}
          </View>
          {!breakdownQuery.isLoading && breakdowns.length === 0 && (
            <Text style={{ color: theme.textSecondary }}>Aucune donnée à afficher pour l&apos;instant.</Text>
          )}
        </>
      )}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  content: { padding: Spacing.four, gap: Spacing.four, paddingBottom: Spacing.six },
  grid: { gap: Spacing.four, width: '100%' },
  stale: { opacity: 0.45 },
  card: { ...Elevation.card, borderRadius: Radius.card, padding: Spacing.four, width: '100%' },
  tabs: { flexDirection: 'row', borderRadius: Radius.pill, padding: 4, gap: 4 },
  tab: { flex: 1, paddingVertical: Spacing.two, borderRadius: Radius.pill, alignItems: 'center' },
  tabText: { fontSize: 13, fontWeight: '700' },
  cardHeader: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', marginBottom: Spacing.three, flexWrap: 'wrap', gap: Spacing.two },
  cardTitle: { fontSize: 13, fontWeight: '700', flexShrink: 1 },
  legend: { flexDirection: 'row', flexWrap: 'wrap', gap: Spacing.three },
  legendItem: { flexDirection: 'row', alignItems: 'center', gap: 4, maxWidth: 140 },
  legendLine: { width: 12, height: 2, borderRadius: 1 },
  legendText: { fontSize: 11 },
  chartArea: { position: 'relative', width: '100%' },
  tooltip: {
    position: 'absolute',
    top: 4,
    ...Elevation.raised,
    borderRadius: Radius.control,
    padding: Spacing.two,
    minWidth: 150,
    maxWidth: 220,
  },
  tooltipDate: { fontSize: 11.5, fontWeight: '700' },
  tooltipTotal: { fontSize: 12, fontWeight: '700', marginTop: 2, fontVariant: ['tabular-nums'] },
  tooltipAccountRow: { flexDirection: 'row', alignItems: 'center', gap: 4, marginTop: 2 },
  tooltipAccountText: { fontSize: 10.5, flexShrink: 1 },
  tooltipOps: { marginTop: Spacing.one, paddingTop: Spacing.one, borderTopWidth: 1, gap: 1 },
  tooltipOpText: { fontSize: 10, fontVariant: ['tabular-nums'] },
  axisRow: { flexDirection: 'row', justifyContent: 'space-between', marginTop: Spacing.one },
  axisLabel: { fontSize: 10 },
  scrubHint: { fontSize: 10, marginTop: 4, fontStyle: 'italic' },
  totalsRow: { marginTop: Spacing.three, gap: 2 },
  currentTotal: { fontSize: 13, fontWeight: '700', fontVariant: ['tabular-nums'] },
  forecastTotal: { fontSize: 12, fontVariant: ['tabular-nums'] },
  pieSubtitle: { fontSize: 11, marginTop: 2, marginBottom: Spacing.three },
  pieRow: { alignItems: 'center', gap: Spacing.four, flexDirection: 'row', flexWrap: 'wrap', justifyContent: 'center' },
  pieLegend: { gap: Spacing.two, flexShrink: 1, alignSelf: 'stretch', minWidth: 180 },
  donut: { width: PIE_SIZE, height: PIE_SIZE },
  pieLegendRow: { flexDirection: 'row', alignItems: 'center', gap: Spacing.two, borderRadius: Radius.control, paddingHorizontal: Spacing.two, paddingVertical: 4, marginHorizontal: -Spacing.two },
  pieLegendDot: { width: 10, height: 10, borderRadius: 5 },
  pieLegendName: { fontSize: 12.5, fontWeight: '600', flexShrink: 1, flexGrow: 1, minWidth: 0 },
  pieLegendAmount: { fontSize: 11.5, fontVariant: ['tabular-nums'] },
});
