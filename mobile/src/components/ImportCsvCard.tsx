import * as DocumentPicker from 'expo-document-picker';
import { useState } from 'react';
import { Pressable, StyleSheet, View } from 'react-native';
import { Text, TextInput } from '@/components/ui/text';
import { ApiError } from '@/api/client';
import { useBankProfiles, useCreateBankProfile, useImportCsv } from '@/api/hooks/useBankProfiles';
import type { AmountMode, ImportBatch } from '@/api/types';
import { OptionPicker, type PickerOption } from '@/components/OptionPicker';
import { Elevation, Radius, Spacing } from '@/constants/theme';
import { useTheme } from '@/hooks/use-theme';

const emptyProfileForm = {
  name: '',
  delimiter: ',',
  encoding: 'utf-8',
  date_format: '%d/%m/%Y',
  decimal_separator: ',',
  amount_mode: 'single' as AmountMode,
  date_col: '',
  label_col: '',
  amount_col: '',
  debit_col: '',
  credit_col: '',
  reference_col: '',
};

const AMOUNT_MODE_OPTIONS: PickerOption<AmountMode>[] = [
  { value: 'single', label: 'Une colonne signée' },
  { value: 'debit_credit', label: 'Colonnes débit / crédit séparées' },
  { value: 'debit_credit_signed', label: 'Colonnes débit / crédit (déjà signées)' },
];

/** Ported from frontend/src/pages/account/ImportTab.tsx: pick a bank
 * profile, pick a CSV file (expo-document-picker — there's no native
 * <input type=file> in RN), upload. The "create a new profile" form is
 * folded behind a toggle instead of always shown — most users only ever
 * need the global "Crédit Mutuel" profile already seeded, a custom one is
 * the rare path. Owner-only, same restriction as web (see AccountLayout's
 * tab list there). */
export function ImportCsvCard({ accountId, isOwner }: { accountId: string; isOwner: boolean }) {
  const theme = useTheme();
  const profilesQuery = useBankProfiles();
  const createProfile = useCreateBankProfile();
  const importCsv = useImportCsv(accountId);

  const [selectedProfileId, setSelectedProfileId] = useState<string | null>(null);
  const [asset, setAsset] = useState<DocumentPicker.DocumentPickerAsset | null>(null);
  const [result, setResult] = useState<ImportBatch | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [showNewProfile, setShowNewProfile] = useState(false);
  const [profileForm, setProfileForm] = useState(emptyProfileForm);
  const [profileError, setProfileError] = useState<string | null>(null);

  if (!isOwner) {
    return (
      <View style={[styles.card, { backgroundColor: theme.surface, borderColor: theme.line }]}>
        <Text style={[styles.title, { color: theme.text }]}>Import CSV</Text>
        <Text style={[styles.hint, { color: theme.textSecondary }]}>
          Seul le propriétaire du compte peut importer un relevé.
        </Text>
      </View>
    );
  }

  const profiles = profilesQuery.data ?? [];
  const effectiveProfileId = selectedProfileId ?? profiles[0]?.id ?? null;
  const profileOptions: PickerOption<string>[] = profiles.map((p) => ({
    value: p.id,
    label: p.global ? `${p.name} (partagé)` : p.name,
  }));

  const pickFile = async () => {
    const picked = await DocumentPicker.getDocumentAsync({ type: ['text/csv', 'text/comma-separated-values', '*/*'] });
    if (picked.canceled || !picked.assets[0]) return;
    setAsset(picked.assets[0]);
    setResult(null);
    setError(null);
  };

  const upload = async () => {
    setError(null);
    setResult(null);
    if (!asset || !effectiveProfileId) {
      setError('Choisissez un profil et un fichier.');
      return;
    }
    try {
      const batch = await importCsv.mutateAsync({ bankProfileId: effectiveProfileId, asset });
      setResult(batch);
      setAsset(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Échec de l'import.");
    }
  };

  const submitProfile = async () => {
    setProfileError(null);
    try {
      const created = await createProfile.mutateAsync({
        name: profileForm.name,
        delimiter: profileForm.delimiter,
        encoding: profileForm.encoding,
        date_format: profileForm.date_format,
        decimal_separator: profileForm.decimal_separator,
        amount_mode: profileForm.amount_mode,
        column_mapping: {
          date: profileForm.date_col,
          label: profileForm.label_col,
          amount: profileForm.amount_mode === 'single' ? profileForm.amount_col || null : null,
          debit: profileForm.amount_mode !== 'single' ? profileForm.debit_col || null : null,
          credit: profileForm.amount_mode !== 'single' ? profileForm.credit_col || null : null,
          reference: profileForm.reference_col || null,
        },
      });
      setProfileForm(emptyProfileForm);
      setSelectedProfileId(created.id);
      setShowNewProfile(false);
    } catch (err) {
      setProfileError(err instanceof ApiError ? err.message : 'Impossible de créer le profil.');
    }
  };

  return (
    <View style={styles.stack}>
      <View style={[styles.card, { backgroundColor: theme.surface, borderColor: theme.line }]}>
        <Text style={[styles.title, { color: theme.text }]}>Import CSV</Text>
        <Text style={[styles.hint, { color: theme.textSecondary }]}>
          Importez un relevé au format CSV en choisissant le profil de votre banque.
        </Text>

        <View style={styles.form}>
          <OptionPicker
            value={effectiveProfileId}
            options={profileOptions}
            onChange={setSelectedProfileId}
            placeholder="Choisir un profil bancaire"
            title="Profil bancaire"
          />

          <Pressable
            onPress={pickFile}
            style={[styles.fileButton, { borderColor: theme.line, backgroundColor: theme.background }]}
          >
            <Text style={{ color: asset ? theme.text : theme.subtle, fontSize: 14 }} numberOfLines={1}>
              {asset ? asset.name : 'Choisir un fichier CSV'}
            </Text>
          </Pressable>

          <Pressable
            onPress={upload}
            disabled={importCsv.isPending || !asset || !effectiveProfileId}
            style={[
              styles.submit,
              { backgroundColor: theme.accent, opacity: importCsv.isPending || !asset || !effectiveProfileId ? 0.6 : 1 },
            ]}
          >
            <Text style={styles.submitText}>{importCsv.isPending ? 'Import en cours…' : 'Importer'}</Text>
          </Pressable>
        </View>

        {error && <Text style={{ color: theme.negative, fontSize: 12.5, marginTop: Spacing.two }}>{error}</Text>}
        {result && (
          <View style={styles.result}>
            <Text style={{ color: theme.positive, fontSize: 13, fontWeight: '700' }}>
              {result.imported_count} opération(s) importée(s) sur {result.row_count}
              {result.duplicate_count > 0 ? ` · ${result.duplicate_count} doublon(s) ignoré(s)` : ''}
            </Text>
            {result.errors.length > 0 && (
              <View style={{ marginTop: Spacing.one }}>
                <Text style={{ color: theme.negative, fontSize: 12 }}>{result.errors.length} erreur(s) :</Text>
                {result.errors.map((e, i) => (
                  <Text key={i} style={{ color: theme.negative, fontSize: 11.5 }}>
                    · {e}
                  </Text>
                ))}
              </View>
            )}
          </View>
        )}
      </View>

      <Pressable onPress={() => setShowNewProfile((v) => !v)} hitSlop={8}>
        <Text style={{ color: theme.accentDark, fontSize: 13, fontWeight: '700' }}>
          {showNewProfile ? 'Annuler' : 'Créer un nouveau profil bancaire'}
        </Text>
      </Pressable>

      {showNewProfile && (
        <View style={[styles.card, { backgroundColor: theme.surface, borderColor: theme.line }]}>
          <Text style={[styles.title, { color: theme.text }]}>Nouveau profil bancaire</Text>
          <View style={styles.form}>
            <LabeledInput
              label="Nom"
              value={profileForm.name}
              onChangeText={(v) => setProfileForm({ ...profileForm, name: v })}
              theme={theme}
            />
            <View style={styles.row}>
              <LabeledInput
                label="Délimiteur"
                value={profileForm.delimiter}
                onChangeText={(v) => setProfileForm({ ...profileForm, delimiter: v })}
                theme={theme}
                style={styles.rowField}
                maxLength={1}
              />
              <LabeledInput
                label="Séparateur décimal"
                value={profileForm.decimal_separator}
                onChangeText={(v) => setProfileForm({ ...profileForm, decimal_separator: v })}
                theme={theme}
                style={styles.rowField}
                maxLength={1}
              />
            </View>
            <LabeledInput
              label="Format de date"
              value={profileForm.date_format}
              onChangeText={(v) => setProfileForm({ ...profileForm, date_format: v })}
              theme={theme}
              placeholder="%d/%m/%Y"
            />
            <OptionPicker
              value={profileForm.amount_mode}
              options={AMOUNT_MODE_OPTIONS}
              onChange={(v) => setProfileForm({ ...profileForm, amount_mode: v })}
              placeholder="Format des montants"
              title="Format des montants"
            />
            <LabeledInput
              label="Colonne date"
              value={profileForm.date_col}
              onChangeText={(v) => setProfileForm({ ...profileForm, date_col: v })}
              theme={theme}
            />
            <LabeledInput
              label="Colonne libellé"
              value={profileForm.label_col}
              onChangeText={(v) => setProfileForm({ ...profileForm, label_col: v })}
              theme={theme}
            />
            {profileForm.amount_mode === 'single' ? (
              <LabeledInput
                label="Colonne montant"
                value={profileForm.amount_col}
                onChangeText={(v) => setProfileForm({ ...profileForm, amount_col: v })}
                theme={theme}
              />
            ) : (
              <View style={styles.row}>
                <LabeledInput
                  label="Colonne débit"
                  value={profileForm.debit_col}
                  onChangeText={(v) => setProfileForm({ ...profileForm, debit_col: v })}
                  theme={theme}
                  style={styles.rowField}
                />
                <LabeledInput
                  label="Colonne crédit"
                  value={profileForm.credit_col}
                  onChangeText={(v) => setProfileForm({ ...profileForm, credit_col: v })}
                  theme={theme}
                  style={styles.rowField}
                />
              </View>
            )}
            <LabeledInput
              label="Colonne référence (optionnel)"
              value={profileForm.reference_col}
              onChangeText={(v) => setProfileForm({ ...profileForm, reference_col: v })}
              theme={theme}
            />
            {profileError && <Text style={{ color: theme.negative, fontSize: 12.5 }}>{profileError}</Text>}
            <Pressable
              onPress={submitProfile}
              disabled={createProfile.isPending || !profileForm.name || !profileForm.date_col || !profileForm.label_col}
              style={[
                styles.submit,
                {
                  backgroundColor: theme.accent,
                  opacity:
                    createProfile.isPending || !profileForm.name || !profileForm.date_col || !profileForm.label_col
                      ? 0.6
                      : 1,
                },
              ]}
            >
              <Text style={styles.submitText}>Enregistrer le profil</Text>
            </Pressable>
          </View>
        </View>
      )}
    </View>
  );
}

function LabeledInput({
  label,
  value,
  onChangeText,
  theme,
  style,
  placeholder,
  maxLength,
}: {
  label: string;
  value: string;
  onChangeText: (v: string) => void;
  theme: ReturnType<typeof useTheme>;
  style?: object;
  placeholder?: string;
  maxLength?: number;
}) {
  return (
    <View style={[styles.field, style]}>
      <Text style={[styles.label, { color: theme.textSecondary }]}>{label}</Text>
      <TextInput
        value={value}
        onChangeText={onChangeText}
        placeholder={placeholder}
        placeholderTextColor={theme.subtle}
        maxLength={maxLength}
        style={[styles.input, { borderColor: theme.line, color: theme.text }]}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  stack: { gap: Spacing.three },
  card: { ...Elevation.card, borderRadius: Radius.card, padding: Spacing.four },
  title: { fontSize: 15, fontWeight: '700' },
  hint: { marginTop: Spacing.one, fontSize: 12.5, lineHeight: 17 },
  form: { marginTop: Spacing.three, gap: Spacing.three },
  fileButton: { borderWidth: 1, borderRadius: 16, paddingHorizontal: Spacing.three, paddingVertical: Spacing.three },
  submit: { ...Elevation.button, borderRadius: Radius.pill, paddingVertical: 12, alignItems: 'center' },
  submitText: { color: '#fff', fontWeight: '700', fontSize: 13.5 },
  result: { marginTop: Spacing.three },
  row: { flexDirection: 'row', gap: Spacing.three },
  rowField: { flex: 1 },
  field: { gap: 4 },
  label: { fontSize: 11, fontWeight: '700' },
  input: { borderWidth: 1, borderRadius: Radius.control, paddingHorizontal: Spacing.three, paddingVertical: 12, fontSize: 14 },
});
