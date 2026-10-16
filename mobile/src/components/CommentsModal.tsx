import { useState } from 'react';
import { ActivityIndicator, Modal, Pressable, ScrollView, StyleSheet, View } from 'react-native';
import { Text, TextInput } from '@/components/ui/text';
import { ApiError } from '@/api/client';
import { useAddComment, useComments } from '@/api/hooks/useOperations';
import { Elevation, Radius, Spacing } from '@/constants/theme';
import { useTheme } from '@/hooks/use-theme';

/** Notes/discussion thread on a single operation — e.g. "à vérifier avec la
 * banque" or "remboursé par Alice le 12". Ported from
 * frontend/src/components/CommentsDialog.tsx (via OperationsTab). */
export function CommentsModal({
  operationId,
  operationLabel,
  onClose,
}: {
  operationId: string;
  operationLabel: string;
  onClose: () => void;
}) {
  const theme = useTheme();
  const commentsQuery = useComments(operationId);
  const addComment = useAddComment(operationId);
  const [body, setBody] = useState('');
  const [error, setError] = useState<string | null>(null);

  const comments = commentsQuery.data ?? [];

  const onSubmit = async () => {
    const trimmed = body.trim();
    if (!trimmed) return;
    setError(null);
    try {
      await addComment.mutateAsync(trimmed);
      setBody('');
    } catch (err) {
      setError(
        err instanceof ApiError
          ? err.message
          : "Impossible d'envoyer le commentaire.",
      );
    }
  };

  return (
    <Modal visible transparent animationType="fade" onRequestClose={onClose}>
      <Pressable style={styles.backdrop} onPress={onClose}>
        <Pressable
          style={[
            styles.sheet,
            { backgroundColor: theme.surface, borderColor: theme.line },
          ]}
        >
          <Text style={[styles.title, { color: theme.text }]} numberOfLines={2}>
            Commentaires · {operationLabel}
          </Text>

          <ScrollView style={styles.list}>
            {commentsQuery.isLoading && (
              <ActivityIndicator
                color={theme.accent}
                style={{ marginTop: Spacing.four }}
              />
            )}
            {!commentsQuery.isLoading && comments.length === 0 && (
              <Text
                style={{
                  color: theme.textSecondary,
                  paddingVertical: Spacing.three,
                }}
              >
                Aucun commentaire.
              </Text>
            )}
            {comments.map((c) => (
              <View
                key={c.id}
                style={[styles.comment, { borderTopColor: theme.line }]}
              >
                <Text style={[styles.commentBody, { color: theme.text }]}>
                  {c.body}
                </Text>
                <Text
                  style={[styles.commentDate, { color: theme.textSecondary }]}
                >
                  {c.created_at.slice(0, 10)}
                </Text>
              </View>
            ))}
          </ScrollView>

          {error && (
            <Text
              style={{
                color: theme.negative,
                fontSize: 12.5,
                marginTop: Spacing.one,
              }}
            >
              {error}
            </Text>
          )}

          <View style={styles.composer}>
            <TextInput
              value={body}
              onChangeText={setBody}
              placeholder="Ajouter un commentaire…"
              placeholderTextColor={theme.subtle}
              style={[
                styles.input,
                { borderColor: theme.line, color: theme.text },
              ]}
              multiline
            />
            <Pressable
              onPress={onSubmit}
              disabled={addComment.isPending || !body.trim()}
              style={[
                styles.sendButton,
                {
                  backgroundColor: theme.accent,
                  opacity: addComment.isPending || !body.trim() ? 0.6 : 1,
                },
              ]}
            >
              <Text style={styles.sendButtonText}>Envoyer</Text>
            </Pressable>
          </View>

          <Pressable onPress={onClose} hitSlop={8} style={styles.closeButton}>
            <Text
              style={[styles.closeButtonText, { color: theme.textSecondary }]}
            >
              Fermer
            </Text>
          </Pressable>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    flex: 1,
    backgroundColor: 'rgba(13, 24, 32, 0.45)',
    justifyContent: 'center',
    padding: Spacing.four,
  },
  sheet: {
    ...Elevation.raised,
    borderRadius: Radius.sheet,
    maxHeight: '80%',
    padding: Spacing.four,
  },
  title: { fontSize: 15, fontWeight: '700', marginBottom: Spacing.two },
  list: { flexGrow: 0, maxHeight: 260 },
  comment: { paddingVertical: Spacing.two, borderTopWidth: 1 },
  commentBody: { fontSize: 13.5 },
  commentDate: { fontSize: 11, marginTop: 2 },
  composer: {
    flexDirection: 'row',
    gap: Spacing.two,
    marginTop: Spacing.three,
    alignItems: 'flex-end',
  },
  input: {
    flex: 1,
    borderWidth: 1,
    borderRadius: Radius.control,
    paddingHorizontal: Spacing.three,
    paddingVertical: Spacing.two,
    fontSize: 13.5,
    maxHeight: 80,
  },
  sendButton: {
    ...Elevation.button,
    borderRadius: Radius.pill,
    paddingVertical: Spacing.two,
    paddingHorizontal: Spacing.three,
  },
  sendButtonText: { color: '#fff', fontWeight: '700', fontSize: 13 },
  closeButton: { alignSelf: 'center', marginTop: Spacing.three },
  closeButtonText: { fontSize: 13, fontWeight: '600' },
});
