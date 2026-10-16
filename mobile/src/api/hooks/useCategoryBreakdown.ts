import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/api/client';
import type { Account, Category, Operation, OperationsPage } from '@/api/types';
import { buildCategoryBreakdown, type CurrencyBreakdown } from '@/domain/categoryBreakdown';
import type { PeriodRange } from '@/domain/period';

/** Prefix of every period's key, so invalidating it (e.g. after a bank
 * sync) refreshes whichever periods are cached. */
export const categoryBreakdownKey = ['categoryBreakdown'] as const;

const PAGE_LIMIT = 500;

/** Every operation of an account in [from, to], following next_cursor —
 * a year or two easily runs past one page. */
async function fetchAllOperations(accountId: string, from: string, to: string): Promise<Operation[]> {
  const operations: Operation[] = [];
  let cursor: string | null = null;
  do {
    const query: string = `date_from=${from}&date_to=${to}&limit=${PAGE_LIMIT}${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`;
    const page: OperationsPage = await api.get<OperationsPage>(`/accounts/${accountId}/operations?${query}`);
    operations.push(...page.operations);
    cursor = page.next_cursor;
  } while (cursor);
  return operations;
}

export function useCategoryBreakdown(range: Pick<PeriodRange, 'from' | 'to'>) {
  const queryClient = useQueryClient();
  return useQuery({
    queryKey: [...categoryBreakdownKey, range.from, range.to],
    queryFn: async (): Promise<CurrencyBreakdown[]> => {
      const [accounts, categories] = await Promise.all([
        queryClient.fetchQuery({ queryKey: ['accounts'], queryFn: () => api.get<Account[]>('/accounts') }),
        queryClient.fetchQuery({ queryKey: ['categories'], queryFn: () => api.get<Category[]>('/categories') }),
      ]);
      const accountsWithOps = await Promise.all(
        accounts.map(async (account) => ({ account, operations: await fetchAllOperations(account.id, range.from, range.to) })),
      );
      return buildCategoryBreakdown(categories, accountsWithOps);
    },
    // Switching period keeps the previous chart on screen (dimmed, see
    // GraphsScreen) until the new one arrives, instead of flashing a spinner.
    placeholderData: keepPreviousData,
  });
}
