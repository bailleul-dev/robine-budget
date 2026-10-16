import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/api/client';
import type { Account, Category, OccurrenceSummary, OperationsPage, PlannedItem, Previsions } from '@/api/types';
import { categoryDisplayName } from '@/domain/category';
import {
  addDays,
  buildDashboardData,
  isoDate,
  TREND_FORECAST_DAYS,
  TREND_PAST_DAYS,
  type DashboardData,
} from '@/domain/dashboard';

export const dashboardKey = ['dashboard'] as const;

// Real operations only exist in the past (forecast points are projections),
// so the fetch stops at today even though the trend window itself runs
// further back and forward — see TREND_PAST_DAYS/TREND_FORECAST_DAYS. Wide
// enough to cover the whole 30-day history for the balance chart's hover
// overlay (see buildDashboardData's operationsByDate), not just a handful
// of "recent activity" rows.
const OPERATIONS_LIMIT = 300;

async function loadAccountData(accountId: string, windowStart: string, today: string, windowEnd: string, monthEnd: string) {
  const [previsions, occurrences, operationsPage] = await Promise.all([
    api.get<Previsions>(`/accounts/${accountId}/previsions?date_from=${windowStart}&date_to=${windowEnd}`),
    api.get<OccurrenceSummary[]>(`/accounts/${accountId}/planned-occurrences?date_to=${monthEnd}`),
    api.get<OperationsPage>(`/accounts/${accountId}/operations?date_from=${windowStart}&date_to=${today}&limit=${OPERATIONS_LIMIT}`),
  ]);
  return { previsions, occurrences, operations: operationsPage.operations };
}

export function useDashboard() {
  const queryClient = useQueryClient();
  return useQuery({
    queryKey: dashboardKey,
    queryFn: async (): Promise<DashboardData> => {
      const accounts = await queryClient.fetchQuery({
        queryKey: ['accounts'],
        queryFn: () => api.get<Account[]>('/accounts'),
      });
      const [allPlannedItems, categories] = await Promise.all([
        queryClient.fetchQuery({ queryKey: ['plannedItems'], queryFn: () => api.get<PlannedItem[]>('/planned-items') }),
        queryClient.fetchQuery({ queryKey: ['categories'], queryFn: () => api.get<Category[]>('/categories') }),
      ]);
      const categoryById = new Map(categories.map((c) => [c.id, c]));
      // A category-mode budget has no label of its own — fall back to its
      // category's display name, and only then to a generic placeholder (an
      // item whose category got deleted, which the UI otherwise has no way
      // to represent).
      const labelById = new Map(
        allPlannedItems.map((item) => {
          if (item.label) return [item.id, item.label] as const;
          const category = item.category_id ? categoryById.get(item.category_id) : undefined;
          return [item.id, category ? categoryDisplayName(category) : 'Budget'] as const;
        }),
      );
      const matchModeById = new Map(allPlannedItems.map((item) => [item.id, item.match_mode] as const));

      const today = new Date();
      // Matches the trend chart's own window (30 days back, 60 forward) —
      // wider than the upcoming-items window strictly needs, but the trend
      // needs the history to draw the "real" half of the line.
      const windowStart = isoDate(addDays(today, -TREND_PAST_DAYS));
      const windowEnd = isoDate(addDays(today, TREND_FORECAST_DAYS));
      const monthEnd = isoDate(new Date(today.getFullYear(), today.getMonth() + 1, 0));

      const results = await Promise.all(
        accounts.map(async (account) => {
          const data = await loadAccountData(account.id, windowStart, isoDate(today), windowEnd, monthEnd);
          return { ...data, account };
        }),
      );

      return buildDashboardData(
        results.map((r) => ({
          account: r.account,
          previsions: r.previsions,
          plannedItemLabels: labelById,
          plannedItemMatchModes: matchModeById,
          occurrences: r.occurrences,
          operations: r.operations,
        })),
      );
    },
  });
}
