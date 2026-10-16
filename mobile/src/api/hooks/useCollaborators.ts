import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/api/client';
import type { Collaborator, InviteInfo } from '@/api/types';

export const collaboratorsKey = ['collaborators'] as const;

/** Public info about a share link (owner's name + status) — fetched before
 * the visitor is signed in, so it backs the invite landing page
 * (app/invite/[token].tsx). */
export function useInviteInfo(token: string) {
  return useQuery({
    queryKey: ['inviteInfo', token],
    queryFn: () => api.get<InviteInfo>(`/invites/${token}`),
    retry: false,
  });
}

export function useRedeemInvite() {
  return useMutation({
    mutationFn: (token: string) => api.post<{ owner_display_name: string }>(`/invites/${token}/redeem`),
  });
}

/** Every share link the current user has issued for their own accounts —
 * pending, accepted, or revoked — backs the "Partage" section in Réglages. */
export function useCollaborators() {
  return useQuery({ queryKey: collaboratorsKey, queryFn: () => api.get<Collaborator[]>('/collaborators') });
}

export function useCreateCollaboratorInvite() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (email: string) => api.post<Collaborator>('/collaborators', { email }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: collaboratorsKey }),
  });
}

export function useRevokeCollaborator() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.delete<Collaborator>(`/collaborators/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: collaboratorsKey }),
  });
}
