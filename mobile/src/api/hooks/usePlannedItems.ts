import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/api/client';
import type {
  Occurrence,
  PlannedItem,
  PlannedItemKind,
  PlannedItemMatchMode,
  RecurrenceType,
  RecurringSuggestion,
} from '@/api/types';

export const plannedItemsKey = ['plannedItems'] as const;

/** Every budget (label-based recurring charge/income, or category-based
 * envelope) across every account the user can see — backs the global
 * "Budgets" screen (and the dashboard's label lookup, see useDashboard.ts). */
export function useAllPlannedItems() {
  return useQuery({ queryKey: plannedItemsKey, queryFn: () => api.get<PlannedItem[]>('/planned-items') });
}

export const recurringSuggestionsKey = ['recurringSuggestions'] as const;

/** Labels in this account's operation history that repeat on a regular
 * weekly or monthly schedule but aren't tracked by any planned item yet —
 * see RecurringDetectionUseCase on the backend. Purely advisory: turning
 * one into an actual item still goes through useCreatePlannedItem (see
 * CreateRecurringItemModal). Ported from frontend's useRecurringSuggestions. */
export function useRecurringSuggestions(accountId: string) {
  return useQuery({
    queryKey: [...recurringSuggestionsKey, accountId],
    queryFn: () => api.get<RecurringSuggestion[]>(`/accounts/${accountId}/recurring-suggestions`),
    enabled: !!accountId,
  });
}

export type CreatePlannedItemInput = {
  accountId: string;
  kind: PlannedItemKind;
  matchMode: PlannedItemMatchMode;
  /** Required when matchMode is 'label', ignored (sent as null) otherwise —
   * a category-mode budget matches by categoryId instead. */
  label: string | null;
  amount: string;
  startDate: string;
  recurrenceType: RecurrenceType;
  occurrenceCount: number | null;
  intervalWeeks: number | null;
  dayOfMonth: number | null;
  categoryId: string | null;
};

export function useCreatePlannedItem() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: CreatePlannedItemInput) =>
      api.post<PlannedItem>('/planned-items', {
        account_id: input.accountId,
        kind: input.kind,
        match_mode: input.matchMode,
        label: input.matchMode === 'label' ? input.label : null,
        amount: input.amount,
        start_date: input.startDate,
        recurrence_type: input.recurrenceType,
        occurrence_count: input.occurrenceCount,
        interval_weeks: input.intervalWeeks,
        day_of_month: input.dayOfMonth,
        category_id: input.categoryId,
        // No UI toggle for this (matching web) — every item created here is
        // active. Omitting it decodes as false, which makes
        // GenerateOccurrencesUseCase silently no-op for the item forever.
        active: true,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: plannedItemsKey });
      // The new item's label is now tracked, so it should drop out of any
      // account's recurring suggestions.
      queryClient.invalidateQueries({ queryKey: recurringSuggestionsKey });
    },
  });
}

/** A single item's own occurrences, each carrying actual_amount for a
 * category-mode budget (the live sum spent/received in its category so far
 * this period — see backend's ReconcileUseCase.CategoryBudgetActuals) or
 * reconciled_operation_id for a label-mode item. Used to show a budget's
 * progress bar (see BudgetProgress in (app)/planned-items.tsx). */
export function usePlannedItemOccurrences(itemId: string) {
  return useQuery({
    queryKey: ['plannedItemOccurrences', itemId],
    queryFn: () => api.get<Occurrence[]>(`/planned-items/${itemId}/occurrences`),
    enabled: !!itemId,
  });
}

export function useGeneratePlannedItemOccurrences() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { itemId: string; monthsAhead?: number }) =>
      api.post<Occurrence[]>(`/planned-items/${input.itemId}/generate-occurrences?months_ahead=${input.monthsAhead ?? 12}`),
    // The backend may have just auto-reconciled (and categorized) existing
    // operations against the newly generated occurrences — refresh anything
    // that could show that. Same keys as web's equivalent hook.
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['operations'] });
      queryClient.invalidateQueries({ queryKey: ['dashboard'] });
      queryClient.invalidateQueries({ queryKey: ['categoryBreakdown'] });
      queryClient.invalidateQueries({ queryKey: ['plannedItemOccurrences'] });
    },
  });
}
