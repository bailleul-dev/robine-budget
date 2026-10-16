import { useQuery } from "@tanstack/react-query";
import { Stack, useLocalSearchParams, useRouter } from "expo-router";
import { useState } from "react";
import { ActivityIndicator, Pressable, ScrollView, SectionList, StyleSheet, View } from "react-native";
import { Text } from "@/components/ui/text";
import { api, ApiError } from "@/api/client";
import { useDeleteAccount } from "@/api/hooks/useAccounts";
import { useBalanceCheckpoints } from "@/api/hooks/useBalanceCheckpoints";
import { useCategories } from "@/api/hooks/useCategories";
import { useAutoCategorize, useInfiniteOperations, useSetOperationCategory } from "@/api/hooks/useOperations";
import { useRecurringSuggestions } from "@/api/hooks/usePlannedItems";
import type {
  Account,
  Operation,
  RecurringSuggestion,
} from "@/api/types";
import { BalanceCheckpointsCard } from "@/components/BalanceCheckpointsCard";
import { CategoryChip } from "@/components/CategoryChip";
import { CommentsModal } from "@/components/CommentsModal";
import { CreateRecurringItemModal } from "@/components/CreateRecurringItemModal";
import { ImportCsvCard } from "@/components/ImportCsvCard";
import { NavIcon, NavIcons } from "@/components/nav-icon";
import { ReconciliationCard } from "@/components/ReconciliationCard";
import { SplitOperationModal } from "@/components/SplitOperationModal";
import { Elevation, Radius, Spacing } from "@/constants/theme";
import { Aurora } from "@/components/ui/aurora";
import { formatMoney } from "@/domain/money";
import { groupOperationsByMonth } from "@/domain/operations";
import { useTabBarClearance } from "@/hooks/use-tab-bar-clearance";
import { useTheme } from "@/hooks/use-theme";
import { confirmAsync } from "@/lib/confirm";

type Tab = "operations" | "import" | "previsions" | "detection" | "reconciliation";
const ALL_TABS: { key: Tab; label: string; ownerOnly?: boolean }[] = [
  { key: "operations", label: "Opérations" },
  { key: "import", label: "Import", ownerOnly: true },
  { key: "previsions", label: "Prévisions" },
  { key: "detection", label: "Détection" },
  { key: "reconciliation", label: "Réconciliation" },
];

/** Turns a suggestion into the shape CreateRecurringItemModal expects — it
 * only reads label/amount/date/category_id, so a synthetic Operation (there
 * being no single real one behind a recurring pattern) is enough. Amount's
 * sign carries the suggestion's kind, matching how the modal derives kind
 * from a real operation's amount. Ported from web's OperationsTab. */
function suggestionAsOperation(suggestion: RecurringSuggestion): Operation {
  const amount =
    Number(suggestion.suggested_amount) *
    (suggestion.kind === "charge" ? -1 : 1);
  return {
    id: suggestion.last_operation_id,
    date: suggestion.anchor_date,
    label: suggestion.label,
    amount: amount.toFixed(2),
    balance_after: null,
    raw_reference: null,
    category_id: suggestion.category_id,
    splits: [],
  };
}

/** Categorize a row (plain or split across several categories), comment on
 * it, or turn one into a recurring item. Operations render in a
 * SectionList (virtualized, same as FlatList) grouped by calendar month
 * with a sticky header per month (see domain/operations.ts), loading a
 * further page (see useInfiniteOperations) as the list nears the end of
 * what's loaded so far. The balance checkpoint and reconciliation features
 * live in their own tabs (Prévisions/Réconciliation, mirroring web's
 * AccountLayout tabs), with the account's current balance pinned above the
 * tabs so it stays visible whichever one
 * is open. */

/** "-9.43" (API decimal string) → "-9,43" with French grouping. U+202F is
 * swapped for a regular no-break space: Manrope has no glyph for it. */
export default function AccountOperationsScreen() {
  const { accountId } = useLocalSearchParams<{ accountId: string }>();
  const theme = useTheme();
  const tabBarClearance = useTabBarClearance();
  const router = useRouter();
  const [tab, setTab] = useState<Tab>("operations");

  const accountQuery = useQuery({
    queryKey: ["accounts", accountId],
    queryFn: () => api.get<Account>(`/accounts/${accountId}`),
    enabled: !!accountId,
  });
  const currency = accountQuery.data?.currency;
  const deleteAccount = useDeleteAccount();
  const onDeleteAccount = async () => {
    const account = accountQuery.data;
    if (!account) return;
    const confirmed = await confirmAsync(
      "Supprimer le compte",
      `Supprimer le compte « ${account.name} » ainsi que toutes ses opérations ? Cette action est irréversible.`,
      "Supprimer",
    );
    if (!confirmed) return;
    await deleteAccount.mutateAsync(account.id);
    router.replace("/");
  };
  const operationsQuery = useInfiniteOperations(accountId ?? "");
  const checkpointsQuery = useBalanceCheckpoints(accountId ?? "");
  const categoriesQuery = useCategories();
  const setOperationCategory = useSetOperationCategory(accountId ?? "");
  const autoCategorize = useAutoCategorize(accountId ?? "");
  const [autoCategorizeMessage, setAutoCategorizeMessage] = useState<string | null>(null);
  const suggestionsQuery = useRecurringSuggestions(accountId);
  const [recurringTarget, setRecurringTarget] = useState<Operation | null>(
    null,
  );
  const [suggestionTarget, setSuggestionTarget] =
    useState<RecurringSuggestion | null>(null);
  const [splitTarget, setSplitTarget] = useState<Operation | null>(null);
  const [commentsTarget, setCommentsTarget] = useState<Operation | null>(null);
  const [dismissedLabels, setDismissedLabels] = useState<Set<string>>(
    new Set(),
  );

  const operations = operationsQuery.data?.pages.flatMap((page) => page.operations) ?? [];
  const operationSections = groupOperationsByMonth(operations);
  const categories = categoriesQuery.data ?? [];
  const suggestions = (suggestionsQuery.data ?? []).filter(
    (s) => !dismissedLabels.has(s.label),
  );
  // Defaults to true while the account is still loading, matching the web
  // app's AccountLayout — avoids the checkpoints form flashing in and out
  // for the (much more common) owner case.
  const isOwner = accountQuery.data
    ? accountQuery.data.owner_display_name === null
    : true;

  // Operations are returned newest-first, so the latest one's balance_after
  // is the account's current balance; falls back to the most recent
  // checkpoint for a brand-new account with no operations yet.
  const currentBalance =
    operations[0]?.balance_after ?? checkpointsQuery.data?.[0]?.balance ?? null;

  const tabs = ALL_TABS.filter((t) => !t.ownerOnly || isOwner);

  const suggestionsCard = suggestions.length > 0 && (
    <View
      style={[
        styles.suggestionsCard,
        { backgroundColor: theme.surface, borderColor: theme.line },
      ]}
    >
      <Text style={[styles.suggestionsTitle, { color: theme.text }]}>
        Élements récurrents suggérés
      </Text>
      <Text style={[styles.suggestionsHint, { color: theme.textSecondary }]}>
        Ces libellés reviennent régulièrement dans vos opérations.
      </Text>
      {suggestions.map((s) => (
        <View
          key={s.label}
          style={[styles.suggestionRow, { borderTopColor: theme.line }]}
        >
          <Text
            style={[styles.label, { color: theme.text }]}
            numberOfLines={1}
          >
            {s.label}
          </Text>
          <View style={styles.suggestionMeta}>
            <Text style={[styles.date, { color: theme.textSecondary }]} numberOfLines={1}>
              {s.recurrence_type === "every_x_weeks"
                ? `Toutes les ${s.interval_weeks} semaines`
                : "Tous les mois"}{" "}
              · vu {s.occurrence_count} fois
            </Text>
            <View style={styles.suggestionActions}>
              <Pressable onPress={() => setSuggestionTarget(s)} hitSlop={8}>
                <Text
                  style={[styles.suggestionAction, { color: theme.accentDark }]}
                >
                  Créer
                </Text>
              </Pressable>
              <Pressable
                onPress={() =>
                  setDismissedLabels((prev) => new Set(prev).add(s.label))
                }
                hitSlop={8}
              >
                <Text style={[styles.suggestionAction, { color: theme.subtle }]}>
                  Ignorer
                </Text>
              </Pressable>
            </View>
          </View>
        </View>
      ))}
    </View>
  );

  const operationsList = (
    <SectionList
      sections={operationSections}
      keyExtractor={(item) => item.id}
      style={styles.listOuter}
      contentContainerStyle={[styles.list, { paddingBottom: Spacing.four + tabBarClearance }]}
      stickySectionHeadersEnabled
      renderSectionHeader={({ section }) => (
        <View style={[styles.monthHeader, { backgroundColor: theme.background }]}>
          <Text style={[styles.monthHeaderText, { color: theme.textSecondary }]}>{section.title}</Text>
        </View>
      )}
      ListHeaderComponent={
        <>
          <View style={styles.autoCategorizeRow}>
            <Pressable
              onPress={async () => {
                setAutoCategorizeMessage(null);
                try {
                  const result = await autoCategorize.mutateAsync();
                  setAutoCategorizeMessage(
                    result.categorized_count > 0
                      ? `${result.categorized_count} opération(s) catégorisée(s).`
                      : "Rien à catégoriser pour l'instant.",
                  );
                } catch (err) {
                  setAutoCategorizeMessage(
                    err instanceof ApiError ? err.message : "Échec de la catégorisation automatique.",
                  );
                }
              }}
              disabled={autoCategorize.isPending}
              style={[
                styles.autoCategorizeButton,
                { borderColor: theme.line, backgroundColor: theme.surface, opacity: autoCategorize.isPending ? 0.6 : 1 },
              ]}
            >
              {autoCategorize.isPending ? (
                <ActivityIndicator size="small" color={theme.text} />
              ) : (
                <NavIcon name={NavIcons.categories} color={theme.textSecondary} size={15} />
              )}
              <Text style={[styles.autoCategorizeText, { color: theme.text }]}>
                Catégoriser automatiquement
              </Text>
            </Pressable>
            {autoCategorizeMessage && (
              <Text style={[styles.autoCategorizeMessage, { color: theme.textSecondary }]}>
                {autoCategorizeMessage}
              </Text>
            )}
          </View>
          <Text style={[styles.hint, { color: theme.textSecondary }]}>
            Touchez une opération pour la transformer en élément récurrent.
          </Text>
        </>
      }
      ListEmptyComponent={
        !operationsQuery.isLoading ? (
          <Text
            style={{
              color: theme.textSecondary,
              paddingVertical: Spacing.four,
            }}
          >
            Aucune opération pour l&apos;instant.
          </Text>
        ) : null
      }
      renderItem={({ item }) => {
        const isNegative = Number(item.amount) < 0;
        return (
          <Pressable
            onPress={() => setRecurringTarget(item)}
            style={({ pressed }) => [
              styles.row,
              { backgroundColor: theme.surface, transform: [{ scale: pressed ? 0.99 : 1 }] },
            ]}
          >
            <View style={[styles.flowBubble, { backgroundColor: isNegative ? theme.negativeBg : theme.positiveBg }]}>
              <Text style={[styles.flowArrow, { color: isNegative ? theme.negative : theme.positive }]}>
                {isNegative ? "↑" : "↓"}
              </Text>
            </View>
            <View style={styles.rowMain}>
              <View style={styles.rowTop}>
                <Text
                  style={[styles.label, { color: theme.text }]}
                  numberOfLines={1}
                >
                  {item.label}
                </Text>
                <Text
                  style={[
                    styles.amount,
                    { color: isNegative ? theme.text : theme.positive },
                  ]}
                >
                  {isNegative ? "" : "+"}
                  {formatMoney(item.amount, currency)}
                </Text>
              </View>
              <View style={styles.rowBottom}>
                <Text style={[styles.date, { color: theme.textSecondary }]}>
                  {item.date}
                </Text>
                <View style={styles.rowBottomRight}>
                  <Pressable
                    onPress={(e) => {
                      e.stopPropagation();
                      setCommentsTarget(item);
                    }}
                    hitSlop={8}
                  >
                    <NavIcon
                      name={NavIcons.comment}
                      color={theme.textSecondary}
                      size={15}
                    />
                  </Pressable>
                  <CategoryChip
                    categories={categories}
                    categoryId={item.category_id}
                    splits={item.splits}
                    onSelect={(categoryId) =>
                      setOperationCategory.mutate({
                        operationId: item.id,
                        categoryId,
                      })
                    }
                    onOpenSplit={() => setSplitTarget(item)}
                  />
                </View>
              </View>
            </View>
          </Pressable>
        );
      }}
      onEndReached={() => {
        if (operationsQuery.hasNextPage && !operationsQuery.isFetchingNextPage) {
          operationsQuery.fetchNextPage();
        }
      }}
      onEndReachedThreshold={0.5}
      ListFooterComponent={
        operationsQuery.isFetchingNextPage ? (
          <ActivityIndicator color={theme.accent} style={{ paddingVertical: Spacing.four }} />
        ) : null
      }
    />
  );

  const isNegativeBalance = currentBalance !== null && Number(currentBalance) < 0;

  return (
    <View style={[styles.container, { backgroundColor: theme.background }]}>
      <Stack.Screen
        options={{
          title: accountQuery.data?.name ?? "Compte",
          // Only the account's actual owner can delete it — a collaborator
          // sees no button at all, same as the (now removed) accounts list
          // screen only showed one for accounts with no owner_display_name.
          headerRight: accountQuery.data && !accountQuery.data.owner_display_name
            ? () => (
                <Pressable onPress={onDeleteAccount} disabled={deleteAccount.isPending} hitSlop={8} style={{ padding: Spacing.one }}>
                  <NavIcon name={NavIcons.delete} color={theme.negative} size={20} />
                </Pressable>
              )
            : undefined,
        }}
      />

      <View style={styles.balanceBar}>
        <Aurora />
        <Text style={[styles.balanceLabel, { color: theme.textSecondary }]}>
          Solde actuel
        </Text>
        <Text
          style={[
            styles.balanceValue,
            { color: isNegativeBalance ? theme.negative : theme.text },
          ]}
        >
          {currentBalance !== null ? formatMoney(currentBalance, currency) : "—"}
        </Text>
      </View>

      <ScrollView
        horizontal
        showsHorizontalScrollIndicator={false}
        style={styles.tabBar}
        contentContainerStyle={styles.tabBarContent}
      >
        {tabs.map((t) => (
          <Pressable
            key={t.key}
            onPress={() => setTab(t.key)}
            style={[
              styles.tabButton,
              { backgroundColor: tab === t.key ? theme.accent : theme.surface },
            ]}
          >
            <Text
              style={[
                styles.tabLabel,
                { color: tab === t.key ? theme.onAccent : theme.textSecondary },
              ]}
            >
              {t.label}
              {t.key === "detection" && suggestions.length > 0 ? ` (${suggestions.length})` : ""}
            </Text>
          </Pressable>
        ))}
      </ScrollView>

      {operationsQuery.isLoading && (
        <ActivityIndicator
          color={theme.accent}
          style={{ marginTop: Spacing.four }}
        />
      )}
      {operationsQuery.isError && (
        <Text style={[styles.error, { color: theme.negative }]}>
          Impossible de charger les opérations.
        </Text>
      )}

      {tab === "operations" && operationsList}
      {tab === "import" && accountId && (
        <ScrollView style={styles.tabPane} contentContainerStyle={[styles.tabPaneContent, { paddingBottom: Spacing.four + tabBarClearance }]}>
          <ImportCsvCard accountId={accountId} isOwner={isOwner} />
        </ScrollView>
      )}
      {tab === "previsions" && accountId && (
        <ScrollView style={styles.tabPane} contentContainerStyle={[styles.tabPaneContent, { paddingBottom: Spacing.four + tabBarClearance }]}>
          <BalanceCheckpointsCard accountId={accountId} currency={currency} isOwner={isOwner} />
        </ScrollView>
      )}
      {tab === "detection" && (
        <ScrollView style={styles.tabPane} contentContainerStyle={[styles.tabPaneContent, { paddingBottom: Spacing.four + tabBarClearance }]}>
          {suggestions.length > 0 ? (
            suggestionsCard
          ) : (
            <Text style={{ color: theme.textSecondary }}>
              Aucun libellé récurrent détecté pour l&apos;instant.
            </Text>
          )}
        </ScrollView>
      )}
      {tab === "reconciliation" && accountId && (
        <ScrollView style={styles.tabPane} contentContainerStyle={[styles.tabPaneContent, { paddingBottom: Spacing.four + tabBarClearance }]}>
          <ReconciliationCard accountId={accountId} currency={currency} />
        </ScrollView>
      )}

      {accountId && recurringTarget && (
        <CreateRecurringItemModal
          key={`op-${recurringTarget.id}`}
          accountId={accountId}
          operation={recurringTarget}
          categories={categories}
          onClose={() => setRecurringTarget(null)}
        />
      )}
      {accountId && suggestionTarget && (
        <CreateRecurringItemModal
          key={`suggestion-${suggestionTarget.label}`}
          accountId={accountId}
          operation={suggestionAsOperation(suggestionTarget)}
          categories={categories}
          initialRecurrenceType={suggestionTarget.recurrence_type}
          initialIntervalWeeks={suggestionTarget.interval_weeks}
          onClose={() => setSuggestionTarget(null)}
        />
      )}
      {accountId && splitTarget && (
        <SplitOperationModal
          key={`split-${splitTarget.id}`}
          accountId={accountId}
          currency={currency}
          operation={splitTarget}
          categories={categories}
          onClose={() => setSplitTarget(null)}
        />
      )}
      {commentsTarget && (
        <CommentsModal
          key={`comments-${commentsTarget.id}`}
          operationId={commentsTarget.id}
          operationLabel={commentsTarget.label}
          onClose={() => setCommentsTarget(null)}
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  error: { marginTop: Spacing.three, marginHorizontal: Spacing.four },
  balanceBar: {
    overflow: "hidden",
    paddingHorizontal: Spacing.four,
    paddingTop: Spacing.four,
    paddingBottom: Spacing.three,
    gap: 2,
  },
  balanceLabel: { fontSize: 13.5, fontWeight: "600" },
  balanceValue: { fontSize: 38, fontWeight: "800", letterSpacing: -1.2, fontVariant: ["tabular-nums"] },
  tabBar: { flexGrow: 0, flexShrink: 0 },
  tabBarContent: {
    flexDirection: "row",
    alignItems: "center",
    gap: Spacing.two,
    paddingHorizontal: Spacing.four,
    paddingVertical: Spacing.two,
  },
  tabButton: {
    ...Elevation.card,
    paddingHorizontal: Spacing.three,
    paddingVertical: 9,
    borderRadius: Radius.pill,
  },
  tabLabel: { fontSize: 13.5, fontWeight: "700" },
  listOuter: { flex: 1, width: "100%" },
  list: {
    width: "100%",
    paddingHorizontal: Spacing.four,
    paddingBottom: Spacing.four,
  },
  tabPane: { flex: 1, width: "100%" },
  tabPaneContent: { padding: Spacing.four, gap: Spacing.four },
  autoCategorizeRow: { paddingTop: Spacing.four, gap: Spacing.two },
  autoCategorizeButton: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    alignSelf: "flex-start",
    gap: Spacing.two,
    borderWidth: 1,
    borderRadius: Radius.pill,
    paddingHorizontal: Spacing.three,
    paddingVertical: Spacing.two,
  },
  autoCategorizeText: { fontWeight: "700", fontSize: 13 },
  autoCategorizeMessage: { fontSize: 12 },
  hint: { fontSize: 11.5, paddingBottom: Spacing.two },
  monthHeader: { paddingTop: Spacing.four, paddingBottom: Spacing.two, paddingHorizontal: Spacing.one },
  monthHeaderText: { fontSize: 16, fontWeight: "800", letterSpacing: -0.3, textTransform: "capitalize" },
  suggestionsCard: {
    ...Elevation.card,
    borderRadius: Radius.card,
    padding: Spacing.four,
  },
  suggestionsTitle: { fontSize: 14, fontWeight: "700" },
  suggestionsHint: { fontSize: 11.5, marginTop: 2 },
  suggestionRow: {
    paddingVertical: Spacing.two,
    borderTopWidth: 1,
    marginTop: Spacing.two,
  },
  suggestionMeta: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: Spacing.two,
    marginTop: 4,
  },
  suggestionActions: { flexDirection: "row", alignItems: "center", gap: Spacing.three, flexShrink: 0 },
  suggestionAction: { fontSize: 12, fontWeight: "700" },
  row: {
    ...Elevation.card,
    flexDirection: "row",
    alignItems: "center",
    gap: 12,
    borderRadius: 20,
    paddingHorizontal: 14,
    paddingVertical: 12,
    marginBottom: Spacing.two,
  },
  flowBubble: { width: 40, height: 40, borderRadius: Radius.pill, alignItems: "center", justifyContent: "center" },
  flowArrow: { fontSize: 17, fontWeight: "800" },
  rowMain: { flex: 1, minWidth: 0 },
  rowTop: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: Spacing.three,
  },
  rowBottom: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: Spacing.two,
    marginTop: 4,
  },
  rowBottomRight: {
    flexDirection: "row",
    alignItems: "center",
    gap: Spacing.two,
  },
  label: { fontSize: 14.5, fontWeight: "600", flexShrink: 1 },
  date: { fontSize: 12 },
  amount: { fontSize: 14.5, fontWeight: "700", fontVariant: ["tabular-nums"] },
});
