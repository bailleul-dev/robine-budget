import { useState, type ReactNode } from 'react';
import { ActivityIndicator, Modal, Pressable, ScrollView, StyleSheet, View } from 'react-native';
import { ApiError } from '@/api/client';
import { useCreatePlannedItem, useGeneratePlannedItemOccurrences } from '@/api/hooks/usePlannedItems';
import type { Account, Category, PlannedItemKind, PlannedItemMatchMode, RecurrenceType } from '@/api/types';
import { CategoryIcon } from '@/components/CategoryIcon';
import { CategoryPicker } from '@/components/CategoryPicker';
import { Text, TextInput } from '@/components/ui/text';
import { Elevation, Radius, Spacing } from '@/constants/theme';
import { categoryDisplayName, flattenCategoryTree } from '@/domain/category';
import { colorForSliceIndex } from '@/domain/categoryBreakdown';
import { currencySymbol } from '@/domain/money';
import { useIsDesktop } from '@/hooks/use-is-desktop';
import { useTheme } from '@/hooks/use-theme';

export type BudgetFlavor = 'envelope' | 'recurring';

const pad = (n: number) => String(n).padStart(2, '0');
const isoDate = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;

function startOptions(now = new Date()) {
  return [
    { key: 'today', label: "Aujourd'hui", value: isoDate(now) },
    { key: 'this-month', label: 'Début du mois', value: isoDate(new Date(now.getFullYear(), now.getMonth(), 1)) },
    { key: 'next-month', label: 'Mois prochain', value: isoDate(new Date(now.getFullYear(), now.getMonth() + 1, 1)) },
  ];
}

function prettyDate(iso: string): string {
  const [y, m, d] = iso.split('-').map(Number);
  if (!y || !m || !d) return iso;
  return new Date(y, m - 1, d).toLocaleDateString('fr-FR', { day: 'numeric', month: 'long', year: 'numeric' });
}

/** Accepts "650", "650,5" or "650.50" — people type amounts the French way. */
function parseAmount(raw: string): number | null {
  const n = Number(raw.replace(/\s/g, '').replace(',', '.'));
  return Number.isFinite(n) && n > 0 ? n : null;
}

const FREQUENCIES: { value: RecurrenceType; label: string }[] = [
  { value: 'perpetual', label: 'Chaque mois' },
  { value: 'every_x_weeks', label: 'Toutes les X semaines' },
  { value: 'fixed_count', label: 'Pendant X mois' },
  { value: 'one_off', label: 'Une seule fois' },
];

/** Creating a budget, as a short conversation instead of a ten-field form.
 *
 * Step 1 asks the only question that changes everything else: an
 * envelope (a monthly ceiling for a category, matched by category) or a
 * regular payment (rent, salary, a subscription, matched by label). Step 2
 * then shows just that flavor's fields, with sensible defaults already
 * filled in (first account, monthly, starting this month) and dates picked
 * from chips rather than typed. A live sentence at the bottom reads the
 * result back before anything is created.
 *
 * On submit the item is created *and* its next 12 months of occurrences
 * are generated right away (same as CreateRecurringItemModal), so there's
 * no separate "Générer les échéances" step to discover. */
export function NewBudgetSheet({
  accounts,
  categories,
  initialFlavor = null,
  onClose,
  onCreated,
}: {
  accounts: Account[];
  categories: Category[];
  initialFlavor?: BudgetFlavor | null;
  onClose: () => void;
  onCreated: (message: string) => void;
}) {
  const theme = useTheme();
  const isDesktop = useIsDesktop();
  const createItem = useCreatePlannedItem();
  const generateOccurrences = useGeneratePlannedItemOccurrences();

  const [flavor, setFlavor] = useState<BudgetFlavor | null>(initialFlavor);
  const [accountId, setAccountId] = useState(accounts[0]?.id ?? '');
  const [kind, setKind] = useState<PlannedItemKind>('charge');
  const [label, setLabel] = useState('');
  const [amount, setAmount] = useState('');
  const [categoryId, setCategoryId] = useState<string | null>(null);
  const [recurrenceType, setRecurrenceType] = useState<RecurrenceType>('perpetual');
  const [intervalWeeks, setIntervalWeeks] = useState('2');
  const [occurrenceCount, setOccurrenceCount] = useState('12');
  const [dayOfMonth, setDayOfMonth] = useState('');
  const starts = startOptions();
  const [startDate, setStartDate] = useState(starts[1].value);
  const [customDate, setCustomDate] = useState(false);
  const [showMore, setShowMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  // Tinted by position among *all* roots, same as the Catégories screen.
  const allRoots = flattenCategoryTree(categories)
    .filter((e) => e.depth === 0)
    .map((e) => e.category);
  const roots = allRoots.filter((c) => c.name !== 'Income');
  const selectedCategory = categories.find((c) => c.id === categoryId);
  const parsedAmount = parseAmount(amount);
  const account = accounts.find((a) => a.id === accountId);
  const symbol = account ? currencySymbol(account.currency) : '';

  const effectiveMatchMode: PlannedItemMatchMode = flavor === 'envelope' ? 'category' : 'label';
  const effectiveKind: PlannedItemKind = flavor === 'envelope' ? 'charge' : kind;
  const effectiveRecurrence: RecurrenceType = flavor === 'envelope' ? 'perpetual' : recurrenceType;

  const missing: string[] = [];
  if (!accountId) missing.push('un compte');
  if (flavor === 'envelope' && !categoryId) missing.push('une catégorie');
  if (flavor === 'recurring' && !label.trim()) missing.push('un nom');
  if (parsedAmount === null) missing.push('un montant');
  if (!/^\d{4}-\d{2}-\d{2}$/.test(startDate)) missing.push('une date valide');
  if (effectiveRecurrence === 'every_x_weeks' && !(Number(intervalWeeks) > 0)) missing.push('un nombre de semaines');
  if (effectiveRecurrence === 'fixed_count' && !(Number(occurrenceCount) > 0)) missing.push('un nombre de mois');

  const summary = (() => {
    const money =
      parsedAmount !== null
        ? `${parsedAmount
            .toLocaleString('fr-FR', {
              minimumFractionDigits: Number.isInteger(parsedAmount) ? 0 : 2,
              maximumFractionDigits: 2,
            })
            // Manrope has no glyph for fr-FR's U+202F thousands separator.
            .replace(/\u202f/g, '\u00a0')}\u00a0${symbol}`
        : `…\u00a0${symbol}`;
    if (flavor === 'envelope') {
      const name = selectedCategory ? `« ${categoryDisplayName(selectedCategory)} »` : 'cette catégorie';
      return `Au plus ${money} par mois pour ${name}, à partir du ${prettyDate(startDate)}.`;
    }
    const what = label.trim() ? `« ${label.trim()} »` : 'ce paiement';
    const verb = effectiveKind === 'income' ? 'Rentrée de' : 'Sortie de';
    const when = {
      perpetual: `chaque mois${dayOfMonth ? `, le ${dayOfMonth}` : ''}`,
      every_x_weeks: `toutes les ${intervalWeeks || 'X'} semaines`,
      fixed_count: `chaque mois pendant ${occurrenceCount || 'X'} mois`,
      one_off: 'une seule fois',
    }[effectiveRecurrence];
    return `${verb} ${money} pour ${what}, ${when}, à partir du ${prettyDate(startDate)}.`;
  })();

  const onSubmit = async () => {
    if (missing.length > 0 || parsedAmount === null) return;
    setError(null);
    setSubmitting(true);
    try {
      const created = await createItem.mutateAsync({
        accountId,
        kind: effectiveKind,
        matchMode: effectiveMatchMode,
        label: flavor === 'recurring' ? label.trim() : null,
        amount: parsedAmount.toFixed(2),
        startDate,
        recurrenceType: effectiveRecurrence,
        occurrenceCount: effectiveRecurrence === 'fixed_count' ? Number(occurrenceCount) : null,
        intervalWeeks: effectiveRecurrence === 'every_x_weeks' ? Number(intervalWeeks) : null,
        dayOfMonth:
          dayOfMonth && (effectiveRecurrence === 'perpetual' || effectiveRecurrence === 'fixed_count')
            ? Number(dayOfMonth)
            : null,
        categoryId,
      });
      await generateOccurrences.mutateAsync({ itemId: created.id, monthsAhead: 12 });
      onCreated(flavor === 'envelope' ? 'Enveloppe créée 🌊' : 'Budget régulier créé 🌊');
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Impossible de créer le budget.');
    } finally {
      setSubmitting(false);
    }
  };

  const chip = (selected: boolean) => [
    styles.chip,
    selected
      ? { backgroundColor: theme.accent, borderColor: theme.accent }
      : { backgroundColor: theme.surface, borderColor: theme.line },
  ];
  const chipText = (selected: boolean) => [styles.chipText, { color: selected ? theme.onAccent : theme.text }];

  return (
    <Modal visible transparent animationType={isDesktop ? 'fade' : 'slide'} onRequestClose={onClose}>
      <Pressable
        style={[styles.backdrop, isDesktop && styles.backdropDesktop, { backgroundColor: 'rgba(13, 24, 32, 0.45)' }]}
        onPress={onClose}
      >
        <Pressable
          style={[styles.sheet, isDesktop && styles.sheetDesktop, { backgroundColor: theme.background }]}
          onPress={() => {}}
        >
          <View style={styles.header}>
            {flavor && !initialFlavor ? (
              <Pressable onPress={() => setFlavor(null)} hitSlop={10} style={[styles.roundButton, { backgroundColor: theme.surface }]}>
                <Text style={[styles.roundButtonText, { color: theme.text }]}>←</Text>
              </Pressable>
            ) : (
              <View style={styles.roundButtonPlaceholder} />
            )}
            <Text style={[styles.headerTitle, { color: theme.text }]}>
              {flavor === 'envelope' ? 'Nouvelle enveloppe' : flavor === 'recurring' ? 'Paiement régulier' : 'Nouveau budget'}
            </Text>
            <Pressable onPress={onClose} hitSlop={10} style={[styles.roundButton, { backgroundColor: theme.surface }]}>
              <Text style={[styles.roundButtonText, { color: theme.text }]}>✕</Text>
            </Pressable>
          </View>

          <ScrollView contentContainerStyle={styles.body} keyboardShouldPersistTaps="handled">
            {flavor === null && (
              <>
                <Text style={[styles.question, { color: theme.text }]}>Que voulez-vous suivre ?</Text>
                <FlavorCard
                  emoji="🧺"
                  tint={theme.accentPale}
                  title="Une enveloppe"
                  description="Un plafond mensuel pour une catégorie. Lagune additionne vos dépenses et vous dit ce qu'il reste."
                  examples="Courses · Sorties · Shopping"
                  onPress={() => setFlavor('envelope')}
                />
                <FlavorCard
                  emoji="🔁"
                  tint={theme.forecastBg}
                  title="Un paiement régulier"
                  description="Une somme qui revient à date fixe. Lagune la prévoit et la reconnaît quand elle passe."
                  examples="Loyer · Salaire · Abonnements"
                  onPress={() => setFlavor('recurring')}
                />
              </>
            )}

            {flavor === 'recurring' && (
              <>
                <View style={styles.kindRow}>
                  {(
                    [
                      { value: 'charge', emoji: '💸', label: 'Une dépense', bg: theme.negativeBg, fg: theme.negative },
                      { value: 'income', emoji: '💰', label: 'Un revenu', bg: theme.positiveBg, fg: theme.positive },
                    ] as const
                  ).map((o) => {
                    const selected = kind === o.value;
                    return (
                      <Pressable
                        key={o.value}
                        onPress={() => setKind(o.value)}
                        style={[
                          styles.kindOption,
                          { backgroundColor: selected ? o.bg : theme.surface, borderColor: selected ? o.fg : 'transparent' },
                        ]}
                      >
                        <Text style={styles.kindEmoji}>{o.emoji}</Text>
                        <Text style={[styles.kindLabel, { color: selected ? o.fg : theme.textSecondary }]}>{o.label}</Text>
                      </Pressable>
                    );
                  })}
                </View>

                <Step title="Comment s'appelle-t-il ?" hint="Écrivez-le comme sur votre relevé bancaire : Lagune le reconnaîtra tout seul.">
                  <TextInput
                    value={label}
                    onChangeText={setLabel}
                    placeholder="ex. Loyer, Netflix, Salaire"
                    placeholderTextColor={theme.subtle}
                    style={[styles.input, { backgroundColor: theme.surface, color: theme.text, outlineColor: theme.accent }]}
                  />
                </Step>
              </>
            )}

            {flavor === 'envelope' && (
              <Step title="Pour quelle catégorie ?">
                <View style={styles.chipWrap}>
                  {roots.map((c) => {
                    const selected = categoryId === c.id;
                    const tint = colorForSliceIndex(allRoots.indexOf(c));
                    return (
                      <Pressable key={c.id} onPress={() => setCategoryId(c.id)} style={chip(selected)}>
                        <CategoryIcon category={c} allCategories={categories} size={15} color={selected ? theme.onAccent : tint} />
                        <Text style={chipText(selected)}>{categoryDisplayName(c)}</Text>
                      </Pressable>
                    );
                  })}
                </View>
                <View style={styles.subPicker}>
                  <Text style={[styles.hint, { color: theme.textSecondary }]}>Ou plus précis :</Text>
                  <View style={{ flex: 1 }}>
                    <CategoryPicker
                      categories={categories}
                      value={selectedCategory?.parent_id ? categoryId : null}
                      onChange={setCategoryId}
                      placeholder="Choisir une sous-catégorie…"
                    />
                  </View>
                </View>
              </Step>
            )}

            {flavor !== null && (
              <>
                <Step title={flavor === 'envelope' ? 'Combien par mois, au plus ?' : 'Quel montant ?'}>
                  <View style={[styles.amountBox, { backgroundColor: theme.surface }]}>
                    <TextInput
                      value={amount}
                      onChangeText={setAmount}
                      placeholder="0"
                      placeholderTextColor={theme.subtle}
                      keyboardType="decimal-pad"
                      style={[styles.amountInput, { color: theme.text, outlineColor: theme.accent }]}
                    />
                    <Text style={[styles.amountCurrency, { color: theme.textSecondary }]}>{symbol}</Text>
                  </View>
                </Step>

                {flavor === 'recurring' && (
                  <Step title="À quelle fréquence ?">
                    <View style={styles.chipWrap}>
                      {FREQUENCIES.map((f) => (
                        <Pressable key={f.value} onPress={() => setRecurrenceType(f.value)} style={chip(recurrenceType === f.value)}>
                          <Text style={chipText(recurrenceType === f.value)}>{f.label}</Text>
                        </Pressable>
                      ))}
                    </View>
                    {recurrenceType === 'every_x_weeks' && (
                      <InlineNumber prefix="Toutes les" suffix="semaines" value={intervalWeeks} onChange={setIntervalWeeks} />
                    )}
                    {recurrenceType === 'fixed_count' && (
                      <InlineNumber prefix="Pendant" suffix="mois" value={occurrenceCount} onChange={setOccurrenceCount} />
                    )}
                    {(recurrenceType === 'perpetual' || recurrenceType === 'fixed_count') && (
                      <InlineNumber prefix="Le" suffix="du mois (facultatif)" value={dayOfMonth} onChange={setDayOfMonth} placeholder="5" />
                    )}
                  </Step>
                )}

                <Step title={effectiveRecurrence === 'one_off' ? 'Quand ?' : 'À partir de quand ?'}>
                  <View style={styles.chipWrap}>
                    {starts.map((s) => {
                      const selected = !customDate && startDate === s.value;
                      return (
                        <Pressable
                          key={s.key}
                          onPress={() => {
                            setCustomDate(false);
                            setStartDate(s.value);
                          }}
                          style={chip(selected)}
                        >
                          <Text style={chipText(selected)}>{s.label}</Text>
                        </Pressable>
                      );
                    })}
                    <Pressable onPress={() => setCustomDate(true)} style={chip(customDate)}>
                      <Text style={chipText(customDate)}>Autre date…</Text>
                    </Pressable>
                  </View>
                  {customDate && (
                    <TextInput
                      value={startDate}
                      onChangeText={setStartDate}
                      placeholder="AAAA-MM-JJ"
                      placeholderTextColor={theme.subtle}
                      style={[styles.input, { backgroundColor: theme.surface, color: theme.text, outlineColor: theme.accent }]}
                    />
                  )}
                </Step>

                {(accounts.length > 1 || flavor === 'recurring') && (
                  <Pressable onPress={() => setShowMore((v) => !v)} style={styles.moreToggle}>
                    <Text style={[styles.moreToggleText, { color: theme.accentDark }]}>
                      {showMore ? 'Moins d’options' : `Plus d’options · ${account?.name ?? 'compte'}`}
                    </Text>
                  </Pressable>
                )}

                {showMore && (
                  <>
                    {accounts.length > 1 && (
                      <Step title="Sur quel compte ?">
                        <View style={styles.chipWrap}>
                          {accounts.map((a) => (
                            <Pressable key={a.id} onPress={() => setAccountId(a.id)} style={chip(accountId === a.id)}>
                              <Text style={chipText(accountId === a.id)} numberOfLines={1}>
                                {a.name}
                              </Text>
                            </Pressable>
                          ))}
                        </View>
                      </Step>
                    )}
                    {flavor === 'recurring' && (
                      <Step title="Ranger dans une catégorie" hint="Facultatif.">
                        <CategoryPicker categories={categories} value={categoryId} onChange={setCategoryId} placeholder="Aucune" />
                      </Step>
                    )}
                  </>
                )}
              </>
            )}
          </ScrollView>

          {flavor !== null && (
            <View style={[styles.footer, { borderTopColor: theme.line }]}>
              <View style={[styles.summary, { backgroundColor: theme.forecastBg }]}>
                <Text style={[styles.summaryText, { color: theme.forecastText }]}>{summary}</Text>
              </View>
              {error && <Text style={[styles.hint, { color: theme.negative }]}>{error}</Text>}
              <Pressable
                onPress={onSubmit}
                disabled={submitting || missing.length > 0}
                style={({ pressed }) => [
                  styles.submit,
                  {
                    backgroundColor: missing.length > 0 ? theme.surfaceSoft : theme.accent,
                    transform: [{ scale: pressed ? 0.98 : 1 }],
                  },
                  missing.length === 0 && Elevation.button,
                ]}
              >
                {submitting ? (
                  <ActivityIndicator color={theme.onAccent} />
                ) : (
                  <Text style={[styles.submitText, { color: missing.length > 0 ? theme.subtle : theme.onAccent }]}>
                    {missing.length > 0 ? `Il manque ${missing[0]}` : 'Créer'}
                  </Text>
                )}
              </Pressable>
            </View>
          )}
        </Pressable>
      </Pressable>
    </Modal>
  );
}

function FlavorCard({
  emoji,
  tint,
  title,
  description,
  examples,
  onPress,
}: {
  emoji: string;
  tint: string;
  title: string;
  description: string;
  examples: string;
  onPress: () => void;
}) {
  const theme = useTheme();
  return (
    <Pressable
      onPress={onPress}
      style={({ pressed }) => [
        styles.flavorCard,
        { backgroundColor: theme.surface, transform: [{ scale: pressed ? 0.98 : 1 }] },
      ]}
    >
      <View style={[styles.flavorEmojiBubble, { backgroundColor: tint }]}>
        <Text style={styles.flavorEmoji}>{emoji}</Text>
      </View>
      <View style={{ flex: 1, gap: 4 }}>
        <Text style={[styles.flavorTitle, { color: theme.text }]}>{title}</Text>
        <Text style={[styles.flavorDescription, { color: theme.textSecondary }]}>{description}</Text>
        <Text style={[styles.flavorExamples, { color: theme.accentDark }]}>{examples}</Text>
      </View>
      <Text style={[styles.flavorArrow, { color: theme.subtle }]}>›</Text>
    </Pressable>
  );
}

function Step({ title, hint, children }: { title: string; hint?: string; children: ReactNode }) {
  const theme = useTheme();
  return (
    <View style={styles.step}>
      <Text style={[styles.stepTitle, { color: theme.text }]}>{title}</Text>
      {hint && <Text style={[styles.hint, { color: theme.textSecondary }]}>{hint}</Text>}
      {children}
    </View>
  );
}

function InlineNumber({
  prefix,
  suffix,
  value,
  onChange,
  placeholder,
}: {
  prefix: string;
  suffix: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
}) {
  const theme = useTheme();
  return (
    <View style={styles.inlineNumber}>
      <Text style={[styles.inlineText, { color: theme.textSecondary }]}>{prefix}</Text>
      <TextInput
        value={value}
        onChangeText={(v) => onChange(v.replace(/[^0-9]/g, ''))}
        keyboardType="number-pad"
        placeholder={placeholder}
        placeholderTextColor={theme.subtle}
        style={[styles.inlineInput, { backgroundColor: theme.surface, color: theme.text, outlineColor: theme.accent }]}
      />
      <Text style={[styles.inlineText, { color: theme.textSecondary }]}>{suffix}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, justifyContent: 'flex-end' },
  backdropDesktop: { justifyContent: 'center', alignItems: 'center', padding: Spacing.four },
  sheet: {
    ...Elevation.raised,
    borderTopLeftRadius: Radius.sheet,
    borderTopRightRadius: Radius.sheet,
    maxHeight: '92%',
    overflow: 'hidden',
  },
  sheetDesktop: { width: '100%', maxWidth: 540, borderRadius: Radius.sheet, maxHeight: '90%' },
  header: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: Spacing.three,
    paddingTop: Spacing.three,
    paddingBottom: Spacing.two,
  },
  headerTitle: { fontSize: 16, fontWeight: '800' },
  roundButton: { ...Elevation.card, width: 36, height: 36, borderRadius: Radius.pill, alignItems: 'center', justifyContent: 'center' },
  roundButtonPlaceholder: { width: 36, height: 36 },
  roundButtonText: { fontSize: 15, fontWeight: '700' },
  body: { padding: Spacing.four, paddingTop: Spacing.two, gap: Spacing.four },
  question: { fontSize: 26, fontWeight: '800', letterSpacing: -0.8 },
  flavorCard: {
    ...Elevation.card,
    flexDirection: 'row',
    alignItems: 'center',
    gap: Spacing.three,
    borderRadius: Radius.card,
    padding: Spacing.three,
  },
  flavorEmojiBubble: { width: 60, height: 60, borderRadius: 20, alignItems: 'center', justifyContent: 'center' },
  flavorEmoji: { fontSize: 30 },
  flavorTitle: { fontSize: 17, fontWeight: '800', letterSpacing: -0.3 },
  flavorDescription: { fontSize: 13.5, lineHeight: 19, fontWeight: '500' },
  flavorExamples: { fontSize: 12, fontWeight: '700' },
  flavorArrow: { fontSize: 28, fontWeight: '400' },
  kindRow: { flexDirection: 'row', gap: Spacing.two },
  kindOption: {
    ...Elevation.card,
    flex: 1,
    alignItems: 'center',
    gap: 4,
    paddingVertical: Spacing.three,
    borderRadius: Radius.card,
    borderWidth: 2,
  },
  kindEmoji: { fontSize: 26 },
  kindLabel: { fontSize: 14, fontWeight: '800' },
  step: { gap: Spacing.two },
  stepTitle: { fontSize: 17, fontWeight: '800', letterSpacing: -0.3 },
  hint: { fontSize: 12.5, fontWeight: '500', lineHeight: 18 },
  input: { ...Elevation.card, borderRadius: Radius.control, paddingHorizontal: Spacing.three, paddingVertical: 14, fontSize: 16 },
  chipWrap: { flexDirection: 'row', flexWrap: 'wrap', gap: Spacing.two },
  chip: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    borderWidth: 1,
    borderRadius: Radius.pill,
    paddingHorizontal: 14,
    paddingVertical: 9,
    maxWidth: '100%',
  },
  chipText: { fontSize: 13.5, fontWeight: '700' },
  subPicker: { flexDirection: 'row', alignItems: 'center', gap: Spacing.two },
  amountBox: {
    ...Elevation.card,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: Radius.card,
    paddingHorizontal: Spacing.four,
    paddingVertical: Spacing.two,
  },
  amountInput: { flex: 1, borderRadius: Radius.control, paddingHorizontal: Spacing.two, fontSize: 40, fontWeight: '800', letterSpacing: -1.2, textAlign: 'right', minWidth: 60 },
  amountCurrency: { fontSize: 28, fontWeight: '700', marginLeft: Spacing.two, flex: 1 },
  inlineNumber: { flexDirection: 'row', alignItems: 'center', gap: Spacing.two },
  inlineText: { fontSize: 14, fontWeight: '600' },
  inlineInput: {
    ...Elevation.card,
    width: 64,
    textAlign: 'center',
    borderRadius: Radius.control,
    paddingVertical: 10,
    fontSize: 16,
    fontWeight: '700',
  },
  moreToggle: { alignSelf: 'flex-start', paddingVertical: Spacing.one },
  moreToggleText: { fontSize: 13.5, fontWeight: '700' },
  footer: { padding: Spacing.three, gap: Spacing.two, borderTopWidth: 1 },
  summary: { borderRadius: Radius.control, padding: Spacing.three },
  summaryText: { fontSize: 13.5, fontWeight: '600', lineHeight: 19 },
  submit: { borderRadius: Radius.pill, paddingVertical: 15, alignItems: 'center' },
  submitText: { fontSize: 15, fontWeight: '800' },
});
