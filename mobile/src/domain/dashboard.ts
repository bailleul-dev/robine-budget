// Ported from frontend/src/domain/dashboard.ts — same aggregation,
// including the balance-trend chart data (now that react-native-svg is
// pulled in to actually draw it — see app/(app)/graphs.tsx). Everything
// here is pure and client-side, same as the web version: there's no
// dedicated dashboard endpoint, this just combines what /previsions,
// /planned-occurrences and /operations already return.
import type { Account, OccurrenceStatus, PlannedItemMatchMode, Previsions } from '@/api/types';

/** One day's total balance for a currency group, used to draw the trend line. */
export type BalancePoint = { date: string; balance: number; isForecast: boolean };

/** One account's own balance trend within a CurrencyTrend — same per-day
 * series shape as the merged total, so the chart can plot both. */
export type AccountTrend = { accountId: string; accountName: string; points: BalancePoint[] };

export type CurrencyTrend = {
  currency: string;
  points: BalancePoint[];
  currentTotal: number;
  accountTrends: AccountTrend[];
};

export type AccountSummary = { account: Account; currentBalance: number };

export type MonthlyOccurrence = {
  occurrenceId: string;
  dueDate: string;
  label: string;
  amount: number;
  currency: string;
  accountId: string;
  accountName: string;
  status: OccurrenceStatus;
  reconciledOperationId: string | null;
  /** true for a category-mode budget's occurrence — these aggregate every
   * matching operation automatically (see backend's CategoryBudgetActuals)
   * and never get a manual "Rapprocher" pick-one-operation action. */
  isBudget: boolean;
  /** Only set (and only meaningful) when isBudget is true — the live sum
   * spent/received in the budget's category so far this period. */
  actualAmount: number | null;
};

export type RecentActivity = {
  date: string;
  label: string;
  amount: number;
  currency: string;
  accountName: string;
};

/** One real operation on a given day, keyed by that "YYYY-MM-DD" date in
 * DashboardData.operationsByDate — backs the balance chart's hover/scrub
 * overlay (see (app)/graphs.tsx), which has no other way to show "what
 * happened" at a point on the line beyond the balance value itself. */
export type DailyOperation = { label: string; amount: number; currency: string; accountName: string };

export type DashboardData = {
  accountSummaries: AccountSummary[];
  trends: CurrencyTrend[];
  upcomingNetByCurrency: Record<string, number>;
  monthlyOccurrences: MonthlyOccurrence[];
  overdueRecurringCount: number;
  recentActivity: RecentActivity[];
  operationsByDate: Record<string, DailyOperation[]>;
};

export const TREND_PAST_DAYS = 30;
export const TREND_FORECAST_DAYS = 60;
const UPCOMING_WINDOW_DAYS = 45;
export const MAX_RECENT_ACTIVITY = 8;

export function isoDate(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

function dateOnly(iso: string): Date {
  // Previsions/occurrence dates are plain "YYYY-MM-DD" — parsing via the
  // Date constructor directly would apply local-timezone offsetting to a
  // midnight-UTC value; splitting avoids that.
  const [y, m, d] = iso.slice(0, 10).split('-').map(Number);
  return new Date(y, m - 1, d);
}

export function addDays(d: Date, days: number): Date {
  const copy = new Date(d);
  copy.setDate(copy.getDate() + days);
  return copy;
}

function isAfter(a: Date, b: Date): boolean {
  return a.getTime() > b.getTime();
}

/** Builds a per-day balance series over [windowStart, windowEnd] for one
 * account's previsions by carrying its running balance forward between
 * entries, so same-currency accounts can be summed day-by-day below. */
function dailyBalances(previsions: Previsions, windowStart: Date, windowEnd: Date): Map<number, number> {
  let balance = Number(previsions.opening_balance);
  const entries = previsions.entries; // already sorted ascending by date
  let entryIndex = 0;
  const byDate = new Map<number, number>();
  let cursor = windowStart;
  while (!isAfter(cursor, windowEnd)) {
    while (entryIndex < entries.length && !isAfter(dateOnly(entries[entryIndex].date), cursor)) {
      balance = Number(entries[entryIndex].running_balance);
      entryIndex++;
    }
    byDate.set(cursor.getTime(), balance);
    cursor = addDays(cursor, 1);
  }
  return byDate;
}

export type AccountDashboardInput = {
  account: Account;
  previsions: Previsions;
  plannedItemLabels: Map<string, string>;
  plannedItemMatchModes: Map<string, PlannedItemMatchMode>;
  occurrences: {
    id: string;
    due_date: string;
    expected_amount: string;
    status: OccurrenceStatus;
    reconciled_operation_id: string | null;
    actual_amount: string | null;
    planned_item_id: string;
  }[];
  operations: { date: string; label: string; amount: string }[];
};

export function buildDashboardData(inputs: AccountDashboardInput[]): DashboardData {
  const today = dateOnly(isoDate(new Date()));
  const windowStart = addDays(today, -TREND_PAST_DAYS);
  const windowEnd = addDays(today, TREND_FORECAST_DAYS);
  const upcomingCutoff = addDays(today, UPCOMING_WINDOW_DAYS);
  const monthStart = new Date(today.getFullYear(), today.getMonth(), 1);
  const monthEnd = new Date(today.getFullYear(), today.getMonth() + 1, 0);

  const accountSummaries: AccountSummary[] = [];
  const monthlyOccurrences: MonthlyOccurrence[] = [];
  let overdueRecurringCount = 0;
  const upcomingNetByCurrency: Record<string, number> = {};
  const recentActivity: RecentActivity[] = [];
  const operationsByDate: Record<string, DailyOperation[]> = {};
  const dailyByCurrency = new Map<string, { accountId: string; accountName: string; daily: Map<number, number> }[]>();

  for (const data of inputs) {
    const { account } = data;
    let currentBalance = Number(data.previsions.opening_balance);
    for (const entry of data.previsions.entries) {
      // A checkpoint catch-up ("adjustment") entry is just as real as an
      // actual operation — see backend's PrevisionsUseCase.
      if ((entry.kind === 'actual' || entry.kind === 'adjustment') && !isAfter(dateOnly(entry.date), today)) {
        currentBalance += Number(entry.amount);
      }
    }
    accountSummaries.push({ account, currentBalance });

    for (const entry of data.previsions.entries) {
      if (entry.kind !== 'forecast') continue;
      if (isAfter(dateOnly(entry.date), upcomingCutoff)) continue;
      upcomingNetByCurrency[account.currency] = (upcomingNetByCurrency[account.currency] ?? 0) + Number(entry.amount);
    }

    for (const occ of data.occurrences) {
      const dueDate = dateOnly(occ.due_date);
      const isBudget = data.plannedItemMatchModes.get(occ.planned_item_id) === 'category';
      // A category budget's occurrence never transitions out of 'forecast'
      // (its "actual" is computed live, not reconciled one operation at a
      // time — see CategoryBudgetActuals) - it's never "overdue" the way an
      // unreconciled bill is, so it must never count or show up here.
      const overdue = !isBudget && occ.status === 'forecast' && !isAfter(dueDate, today);
      if (overdue) {
        overdueRecurringCount++;
      }
      // Everything due this month, plus any overdue item from an earlier
      // month that would otherwise be counted above but never shown
      // anywhere actionable.
      if (!overdue && (dueDate < monthStart || dueDate > monthEnd)) continue;
      monthlyOccurrences.push({
        occurrenceId: occ.id,
        dueDate: occ.due_date,
        label: data.plannedItemLabels.get(occ.planned_item_id) ?? 'Budget',
        amount: Number(occ.expected_amount),
        currency: account.currency,
        accountId: account.id,
        accountName: account.name,
        status: occ.status,
        reconciledOperationId: occ.reconciled_operation_id,
        isBudget,
        actualAmount: occ.actual_amount !== null ? Number(occ.actual_amount) : null,
      });
    }

    for (const op of data.operations.slice(0, MAX_RECENT_ACTIVITY)) {
      recentActivity.push({
        date: op.date,
        label: op.label,
        amount: Number(op.amount),
        currency: account.currency,
        accountName: account.name,
      });
    }

    for (const op of data.operations) {
      const day = op.date.slice(0, 10);
      const list = operationsByDate[day] ?? [];
      list.push({ label: op.label, amount: Number(op.amount), currency: account.currency, accountName: account.name });
      operationsByDate[day] = list;
    }

    const list = dailyByCurrency.get(account.currency) ?? [];
    list.push({
      accountId: account.id,
      accountName: account.name,
      daily: dailyBalances(data.previsions, windowStart, windowEnd),
    });
    dailyByCurrency.set(account.currency, list);
  }

  const trends: CurrencyTrend[] = [...dailyByCurrency.entries()].map(([currency, accounts]) => {
    const points: BalancePoint[] = [];
    for (let cursor = windowStart; !isAfter(cursor, windowEnd); cursor = addDays(cursor, 1)) {
      const balance = accounts.reduce((sum, a) => sum + (a.daily.get(cursor.getTime()) ?? 0), 0);
      points.push({ date: isoDate(cursor), balance, isForecast: isAfter(cursor, today) });
    }
    const accountTrends: AccountTrend[] = accounts.map((a) => {
      const accPoints: BalancePoint[] = [];
      for (let cursor = windowStart; !isAfter(cursor, windowEnd); cursor = addDays(cursor, 1)) {
        accPoints.push({
          date: isoDate(cursor),
          balance: a.daily.get(cursor.getTime()) ?? 0,
          isForecast: isAfter(cursor, today),
        });
      }
      return { accountId: a.accountId, accountName: a.accountName, points: accPoints };
    });
    const currentTotal = accountSummaries
      .filter((s) => s.account.currency === currency)
      .reduce((sum, s) => sum + s.currentBalance, 0);
    return { currency, points, currentTotal, accountTrends };
  });

  monthlyOccurrences.sort((a, b) => a.dueDate.localeCompare(b.dueDate));
  recentActivity.sort((a, b) => b.date.localeCompare(a.date));

  return {
    accountSummaries,
    trends,
    upcomingNetByCurrency,
    monthlyOccurrences,
    overdueRecurringCount,
    recentActivity: recentActivity.slice(0, MAX_RECENT_ACTIVITY),
    operationsByDate,
  };
}
