import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/api/client';
import type { Comment, OperationsPage } from '@/api/types';

export const operationsKey = (accountId: string) => ['operations', accountId] as const;
export const commentsKey = (operationId: string) => ['comments', operationId] as const;

const OPERATIONS_PAGE_SIZE = 50;

// Keyset-paginated (see backend's ListPageForAccount) — each page's
// next_cursor feeds the next request until the backend returns null. Same
// queryKey as the mutations below invalidate, so a category change/split/
// auto-categorize correctly refetches this list regardless of how many
// pages have been loaded. FlatList renders every loaded operation itself
// (virtualized — only what's on/near screen actually mounts), this just
// grows how many are loaded as the user scrolls.
export function useInfiniteOperations(accountId: string) {
  return useInfiniteQuery({
    queryKey: operationsKey(accountId),
    queryFn: ({ pageParam }: { pageParam: string | null }) =>
      api.get<OperationsPage>(
        `/accounts/${accountId}/operations?limit=${OPERATIONS_PAGE_SIZE}${pageParam ? `&cursor=${encodeURIComponent(pageParam)}` : ''}`,
      ),
    initialPageParam: null as string | null,
    getNextPageParam: (lastPage) => lastPage.next_cursor,
    enabled: !!accountId,
  });
}

export function useComments(operationId: string | null) {
  return useQuery({
    queryKey: commentsKey(operationId ?? ''),
    queryFn: () => api.get<Comment[]>(`/operations/${operationId}/comments`),
    enabled: operationId !== null,
  });
}

export function useAddComment(operationId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: string) => api.post<Comment>(`/operations/${operationId}/comments`, { body }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: commentsKey(operationId) }),
  });
}

export function useSetOperationCategory(accountId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { operationId: string; categoryId: string | null }) =>
      api.put(`/operations/${input.operationId}/category`, { category_id: input.categoryId }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: operationsKey(accountId) }),
  });
}

export type AutoCategorizeResult = { categorized_count: number };

// Bulk-fills every currently-uncategorized operation in the account it can
// (merchant dictionary + exact-label memory — see backend's
// AutoCategorizeUseCase). New operations already get this automatically on
// import/sync; this is for catching up an account's existing backlog.
export function useAutoCategorize(accountId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.post<AutoCategorizeResult>(`/accounts/${accountId}/operations/auto-categorize`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: operationsKey(accountId) }),
  });
}

// Splits and category_id are mutually exclusive (see backend's SetSplits) —
// setting one clears the other. An empty splits array clears back to plain
// (unsplit) categorization.
export function useSetOperationSplits(accountId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { operationId: string; splits: { categoryId: string; amount: string }[] }) =>
      api.put(`/operations/${input.operationId}/splits`, {
        splits: input.splits.map((s) => ({ category_id: s.categoryId, amount: s.amount })),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: operationsKey(accountId) }),
  });
}
