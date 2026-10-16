import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/api/client';
import type { ConnectedApp } from '@/api/types';

export const connectedAppsKey = ['connectedApps'] as const;

/** Apps authorized to use the MCP endpoint on the user's behalf — backs the
 * "Applications connectées" section in Réglages. */
export function useConnectedApps() {
  return useQuery({ queryKey: connectedAppsKey, queryFn: () => api.get<ConnectedApp[]>('/oauth/grants') });
}

/** Disconnects an app: its tokens stop working immediately. */
export function useRevokeConnectedApp() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.delete<void>(`/oauth/grants/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: connectedAppsKey }),
  });
}
