import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/api/client';
import type { Account } from '@/api/types';
import { dashboardKey } from './useDashboard';

export const accountsKey = ['accounts'] as const;

export function useAccounts() {
  return useQuery({ queryKey: accountsKey, queryFn: () => api.get<Account[]>('/accounts') });
}

// Deletes the account and everything in it (operations, planned items,
// checkpoints, import history — see backend migration 000014's cascade).
// Owner-only and irreversible; the caller is responsible for confirming
// with the user first (see confirmDelete in the accounts list screen).
export function useDeleteAccount() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (accountId: string) => api.delete(`/accounts/${accountId}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: accountsKey });
      queryClient.invalidateQueries({ queryKey: dashboardKey });
    },
  });
}
