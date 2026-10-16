import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/api/client';
import type { BankConnectionStatus, BankSyncResult } from '@/api/types';
import { categoryBreakdownKey } from './useCategoryBreakdown';
import { dashboardKey } from './useDashboard';

export const bankSyncStatusKey = ['bank-sync-status'] as const;

export function useBankSyncStatus() {
  return useQuery({
    queryKey: bankSyncStatusKey,
    queryFn: () => api.get<BankConnectionStatus>('/bank-sync/status'),
  });
}

// A sync (or the first sync a connect triggers) can create new accounts and
// change balances — invalidate everything that reads either, same spirit as
// useBalanceCheckpoints' invalidateAfterCheckpointChange. Status itself is
// worth refetching even after a failed connect: the backend keeps the
// connection row (and so reports "connected") even when the first sync
// failed on e.g. an unconfirmed app-validation — the client retries with
// Sync from then on, not Connect again.
function invalidateAfterBankSync(queryClient: ReturnType<typeof useQueryClient>) {
  queryClient.invalidateQueries({ queryKey: bankSyncStatusKey });
  queryClient.invalidateQueries({ queryKey: ['accounts'] });
  queryClient.invalidateQueries({ queryKey: dashboardKey });
  queryClient.invalidateQueries({ queryKey: categoryBreakdownKey });
}

export function useConnectBank() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { login: string; password: string }) =>
      api.post<BankSyncResult>('/bank-sync/connect', { bank: 'cm', login: input.login, password: input.password }),
    onSettled: () => invalidateAfterBankSync(queryClient),
  });
}

export function useSyncBank() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.post<BankSyncResult>('/bank-sync/sync'),
    onSettled: () => invalidateAfterBankSync(queryClient),
  });
}
