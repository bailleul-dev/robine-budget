import { useState } from 'react';
import { ActivityIndicator, Modal, Pressable, ScrollView, StyleSheet, View } from 'react-native';
import { ApiError } from '@/api/client';
import { useAuth } from '@/auth/AuthContext';
import { Text, TextInput } from '@/components/ui/text';
import { Elevation, Radius, Spacing } from '@/constants/theme';
import { useIsDesktop } from '@/hooks/use-is-desktop';
import { useTheme } from '@/hooks/use-theme';

const CONFIRM_WORD = 'SUPPRIMER';

const WHAT_GOES = [
  'Vos comptes, opérations, budgets et catégories',
  'Votre connexion bancaire et ses identifiants',
  'Les liens de partage que vous avez créés, et vos accès aux comptes partagés avec vous',
];

/** "Supprimer mon compte" for Réglages — in-app account deletion, which the
 * App Store requires of any app that lets people sign up (guideline
 * 5.1.1(v)). Deliberately two steps: a sheet spelling out exactly what
 * disappears, then typing SUPPRIMER to arm the button. Backed by
 * DELETE /api/auth/me (see backend UserRepository.Delete). */
export function DeleteAccountSection() {
  const theme = useTheme();
  const isDesktop = useIsDesktop();
  const { deleteAccount } = useAuth();
  const [open, setOpen] = useState(false);
  const [typed, setTyped] = useState('');
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const armed = typed.trim().toUpperCase() === CONFIRM_WORD;

  const close = () => {
    if (deleting) return;
    setOpen(false);
    setTyped('');
    setError(null);
  };

  const onDelete = async () => {
    if (!armed) return;
    setError(null);
    setDeleting(true);
    try {
      // On success the auth state clears and the app navigates to login on
      // its own — this component unmounts with the rest of Réglages.
      await deleteAccount();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'La suppression a échoué. Réessayez.');
      setDeleting(false);
    }
  };

  return (
    <>
      <Pressable
        onPress={() => setOpen(true)}
        style={({ pressed }) => [styles.entry, { backgroundColor: theme.surface, opacity: pressed ? 0.8 : 1 }]}
      >
        <Text style={[styles.entryText, { color: theme.negative }]}>Supprimer mon compte</Text>
        <Text style={[styles.entryHint, { color: theme.textSecondary }]}>
          Efface définitivement votre compte Lagune et toutes vos données.
        </Text>
      </Pressable>

      <Modal visible={open} transparent animationType={isDesktop ? 'fade' : 'slide'} onRequestClose={close}>
        <Pressable
          style={[styles.backdrop, isDesktop && styles.backdropDesktop, { backgroundColor: 'rgba(13, 24, 32, 0.45)' }]}
          onPress={close}
        >
          <Pressable style={[styles.sheet, isDesktop && styles.sheetDesktop, { backgroundColor: theme.background }]}>
            <ScrollView contentContainerStyle={styles.body} keyboardShouldPersistTaps="handled">
              <View style={[styles.iconBubble, { backgroundColor: theme.negativeBg }]}>
                <Text style={styles.icon}>🗑️</Text>
              </View>
              <Text style={[styles.title, { color: theme.text }]}>Supprimer votre compte ?</Text>
              <Text style={[styles.lead, { color: theme.textSecondary }]}>
                C&apos;est immédiat et définitif : nous ne pourrons rien récupérer ensuite. Seront effacés :
              </Text>
              <View style={[styles.list, { backgroundColor: theme.surface }]}>
                {WHAT_GOES.map((item) => (
                  <View key={item} style={styles.listRow}>
                    <View style={[styles.dot, { backgroundColor: theme.negative }]} />
                    <Text style={[styles.listText, { color: theme.text }]}>{item}</Text>
                  </View>
                ))}
              </View>
              <Text style={[styles.lead, { color: theme.textSecondary }]}>
                Pour confirmer, tapez <Text style={{ fontWeight: '800', color: theme.text }}>{CONFIRM_WORD}</Text>{' '}
                ci-dessous.
              </Text>
              <TextInput
                value={typed}
                onChangeText={setTyped}
                autoCapitalize="characters"
                autoCorrect={false}
                placeholder={CONFIRM_WORD}
                placeholderTextColor={theme.subtle}
                editable={!deleting}
                style={[
                  styles.input,
                  { backgroundColor: theme.surface, color: theme.text, borderColor: armed ? theme.negative : theme.line },
                ]}
              />
              {error && <Text style={[styles.error, { color: theme.negative }]}>{error}</Text>}
            </ScrollView>

            <View style={[styles.footer, { borderTopColor: theme.line }]}>
              <Pressable
                onPress={close}
                disabled={deleting}
                style={[styles.button, { backgroundColor: theme.surfaceSoft }]}
              >
                <Text style={[styles.buttonText, { color: theme.text }]}>Annuler</Text>
              </Pressable>
              <Pressable
                onPress={onDelete}
                disabled={!armed || deleting}
                style={[styles.button, { backgroundColor: armed ? theme.negative : theme.negativeBg }]}
              >
                {deleting ? (
                  <ActivityIndicator color="#ffffff" />
                ) : (
                  <Text style={[styles.buttonText, { color: armed ? '#ffffff' : theme.negative }]}>Supprimer</Text>
                )}
              </Pressable>
            </View>
          </Pressable>
        </Pressable>
      </Modal>
    </>
  );
}

const styles = StyleSheet.create({
  entry: { ...Elevation.card, borderRadius: Radius.card, padding: Spacing.four, gap: 4 },
  entryText: { fontSize: 15, fontWeight: '800' },
  entryHint: { fontSize: 12.5, fontWeight: '500' },
  backdrop: { flex: 1, justifyContent: 'flex-end' },
  backdropDesktop: { justifyContent: 'center', alignItems: 'center', padding: Spacing.four },
  sheet: {
    ...Elevation.raised,
    borderTopLeftRadius: Radius.sheet,
    borderTopRightRadius: Radius.sheet,
    maxHeight: '90%',
    overflow: 'hidden',
  },
  sheetDesktop: { width: '100%', maxWidth: 480, borderRadius: Radius.sheet },
  body: { padding: Spacing.four, gap: Spacing.three },
  iconBubble: { width: 56, height: 56, borderRadius: Radius.pill, alignItems: 'center', justifyContent: 'center' },
  icon: { fontSize: 26 },
  title: { fontSize: 24, fontWeight: '800', letterSpacing: -0.6 },
  lead: { fontSize: 14, fontWeight: '500', lineHeight: 20 },
  list: { ...Elevation.card, borderRadius: Radius.control, padding: Spacing.three, gap: Spacing.two },
  listRow: { flexDirection: 'row', alignItems: 'flex-start', gap: Spacing.two },
  dot: { width: 7, height: 7, borderRadius: Radius.pill, marginTop: 7 },
  listText: { flex: 1, fontSize: 14, fontWeight: '600', lineHeight: 20 },
  input: {
    borderWidth: 1.5,
    borderRadius: Radius.control,
    paddingHorizontal: Spacing.three,
    paddingVertical: 14,
    fontSize: 16,
    fontWeight: '700',
    letterSpacing: 2,
  },
  error: { fontSize: 13, fontWeight: '600' },
  footer: { flexDirection: 'row', gap: Spacing.two, padding: Spacing.three, borderTopWidth: 1 },
  button: { flex: 1, borderRadius: Radius.pill, paddingVertical: 14, alignItems: 'center' },
  buttonText: { fontSize: 15, fontWeight: '800' },
});
