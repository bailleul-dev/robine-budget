import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/api/client';
import type { Category } from '@/api/types';

export const categoriesKey = ['categories'] as const;

export function useCategories() {
  return useQuery({ queryKey: categoriesKey, queryFn: () => api.get<Category[]>('/categories') });
}

export function useCreateCategory() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; parentId: string }) =>
      api.post<Category>('/categories', { name: input.name, parent_id: input.parentId }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: categoriesKey }),
  });
}

export function useUpdateCategory() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { id: string; name: string; parentId: string }) =>
      api.put<Category>(`/categories/${input.id}`, { name: input.name, parent_id: input.parentId }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: categoriesKey }),
  });
}

export function useDeleteCategory() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.delete(`/categories/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: categoriesKey }),
  });
}
