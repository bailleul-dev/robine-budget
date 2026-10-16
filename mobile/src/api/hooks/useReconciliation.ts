import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/api/client';
import type { OperationSummary, ReconciliationSuggestion } from '@/api/types';

export const reconciliationSuggestionsKey = (accountId: string) =>
  ['reconciliation', 'suggestions', accountId] as const;
export const unreconciledOperationsKey = (accountId: string) =>
  ['reconciliation', 'unreconciled', accountId] as const;

export function useReconciliationSuggestions(accountId: string) {
  return useQuery({
    queryKey: reconciliationSuggestionsKey(accountId),
    queryFn: () =>
      api.get<ReconciliationSuggestion[]>(
        `/accounts/${accountId}/reconciliation/suggestions`,
      ),
  });
}

export function useUnreconciledOperations(accountId: string) {
  return useQuery({
    queryKey: unreconciledOperationsKey(accountId),
    queryFn: () =>
      api.get<OperationSummary[]>(
        `/accounts/${accountId}/reconciliation/unreconciled-operations`,
      ),
  });
}

export function useReconcile(accountId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { occurrenceId: string; operationId: string }) =>
      api.post('/reconciliation', {
        occurrence_id: input.occurrenceId,
        operation_id: input.operationId,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: reconciliationSuggestionsKey(accountId),
      });
      queryClient.invalidateQueries({
        queryKey: unreconciledOperationsKey(accountId),
      });
    },
  });
}
