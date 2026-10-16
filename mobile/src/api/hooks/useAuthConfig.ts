import { useQuery } from '@tanstack/react-query';
import { api } from '@/api/client';
import type { AuthConfig } from '@/api/types';

/** Whether Robine ID login is configured server-side — unlike Google (a
 * per-platform client ID baked into the app at build time, see
 * auth/google.ts), Robine is a plain server-redirect flow with nothing to
 * configure client-side, so the only way to know whether to show its
 * button is to ask the backend (GET /api/auth/config). Public endpoint, so
 * this is safe to call before the user is signed in — see login.tsx. */
export function useAuthConfig() {
  return useQuery({ queryKey: ['authConfig'], queryFn: () => api.get<AuthConfig>('/auth/config') });
}
