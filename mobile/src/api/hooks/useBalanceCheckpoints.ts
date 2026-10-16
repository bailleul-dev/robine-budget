import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/api/client';
import type { BalanceCheckpoint } from '@/api/types';
import { dashboardKey } from './useDashboard';

export const balanceCheckpointsKey = (accountId: string) => ['balance-checkpoints', accountId] as const;

export function useBalanceCheckpoints(accountId: string) {
  return useQuery({
    queryKey: balanceCheckpointsKey(accountId),
    queryFn: () => api.get<BalanceCheckpoint[]>(`/accounts/${accountId}/balance-checkpoints`),
    enabled: !!accountId,
  });
}

type CheckpointInput = { asOfDate: string; balance: string; note: string | null };

// A new/changed/deleted checkpoint can shift the opening balance previsions
// (and therefore the dashboard's current balance) anchor on — invalidate
// both, same as frontend/src/api/hooks/useBalanceCheckpoints.ts.
function invalidateAfterCheckpointChange(queryClient: ReturnType<typeof useQueryClient>, accountId: string) {
  queryClient.invalidateQueries({ queryKey: balanceCheckpointsKey(accountId) });
  queryClient.invalidateQueries({ queryKey: dashboardKey });
}

export function useCreateBalanceCheckpoint(accountId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: CheckpointInput) =>
      api.post<BalanceCheckpoint>(`/accounts/${accountId}/balance-checkpoints`, {
        as_of_date: input.asOfDate,
        balance: input.balance,
        note: input.note,
      }),
    onSuccess: () => invalidateAfterCheckpointChange(queryClient, accountId),
  });
}

/** Bootstraps the account's very first checkpoint from a balance the user
 * knows to be true right now — the backend computes the true opening
 * balance by walking it backward across every operation since. Only valid
 * while the account has no checkpoint yet (see
 * ComputeInitialBalanceUseCase on the backend). */
export function useComputeInitialBalance(accountId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: CheckpointInput) =>
      api.post<BalanceCheckpoint>(`/accounts/${accountId}/balance-checkpoints/initial`, {
        as_of_date: input.asOfDate,
        balance: input.balance,
        note: input.note,
      }),
    onSuccess: () => invalidateAfterCheckpointChange(queryClient, accountId),
  });
}

export function useDeleteBalanceCheckpoint(accountId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (checkpointId: string) => api.delete(`/balance-checkpoints/${checkpointId}`),
    onSuccess: () => invalidateAfterCheckpointChange(queryClient, accountId),
  });
}
