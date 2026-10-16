import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { DocumentPickerAsset } from 'expo-document-picker';
import { Platform } from 'react-native';
import { api } from '@/api/client';
import type { BankProfile, ImportBatch } from '@/api/types';

export const bankProfilesKey = ['bankProfiles'] as const;

export function useBankProfiles() {
  return useQuery({ queryKey: bankProfilesKey, queryFn: () => api.get<BankProfile[]>('/bank-profiles') });
}

export function useCreateBankProfile() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (profile: Omit<BankProfile, 'id' | 'global' | 'created_at' | 'updated_at'>) =>
      api.post<BankProfile>('/bank-profiles', profile),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: bankProfilesKey }),
  });
}

// RN's FormData accepts a {uri, name, type} triple for a file part on
// native (iOS/Android); on web it needs a real File/Blob, which
// expo-document-picker only gives us via asset.file there — see
// DocumentPickerAsset's platform-specific fields.
function appendFile(form: FormData, asset: DocumentPickerAsset) {
  if (Platform.OS === 'web' && asset.file) {
    form.append('file', asset.file, asset.name);
    return;
  }
  form.append('file', {
    uri: asset.uri,
    name: asset.name,
    type: asset.mimeType ?? 'text/csv',
  } as unknown as Blob);
}

export function useImportCsv(accountId: string) {
  return useMutation({
    mutationFn: (input: { bankProfileId: string; asset: DocumentPickerAsset }) => {
      const form = new FormData();
      form.append('bank_profile_id', input.bankProfileId);
      appendFile(form, input.asset);
      return api.postForm<ImportBatch>(`/accounts/${accountId}/imports`, form);
    },
  });
}
