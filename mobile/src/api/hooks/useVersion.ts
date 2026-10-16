import { useQuery } from '@tanstack/react-query';
import { api } from '@/api/client';
import type { VersionInfo } from '@/api/types';

export const versionKey = ['version'] as const;

export function useVersion() {
  return useQuery({
    queryKey: versionKey,
    queryFn: () => api.get<VersionInfo>('/version'),
  });
}
