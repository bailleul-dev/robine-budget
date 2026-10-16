import * as Updates from 'expo-updates';
import { useState } from 'react';
import { ActivityIndicator, Platform, Pressable, StyleSheet, View } from 'react-native';
import { Text } from '@/components/ui/text';
import { Elevation, Radius, Spacing } from '@/constants/theme';
import { useTheme } from '@/hooks/use-theme';

/** Over-the-air updates only exist in native release builds pointed at our
 * update server (see app.config.ts). Not on web — expo-updates' web stub
 * reports isEnabled: true, hence the explicit platform check — and not in
 * development, where every check/fetch call rejects. */
export const appUpdatesSupported = Platform.OS !== 'web' && !__DEV__ && Updates.isEnabled;

/** expo-updates' own state machine (it already checks and downloads on
 * launch, see checkAutomatically in app.config.ts), narrowed to what the UI
 * needs. Safe to call everywhere: on unsupported platforms it just reports
 * nothing pending. */
export function useAppUpdate() {
  const state = Updates.useUpdates();
  return {
    supported: appUpdatesSupported,
    isPending: appUpdatesSupported && state.isUpdatePending,
    isDownloading: appUpdatesSupported && state.isDownloading,
    isChecking: appUpdatesSupported && state.isChecking,
    downloadProgress: state.downloadProgress,
    currentlyRunning: state.currentlyRunning,
  };
}

function restart() {
  Updates.reloadAsync().catch(() => {
    // Nothing useful to show: the update still applies on next cold start.
  });
}

/** Shown at the top of Réglages once a newer version has been downloaded
 * in the background: one tap restarts into it instead of waiting for the
 * next cold start. Renders nothing otherwise. */
export function AppUpdateBanner() {
  const theme = useTheme();
  const { isPending, isDownloading, downloadProgress } = useAppUpdate();

  if (isDownloading && !isPending) {
    const percent = downloadProgress ? Math.round(downloadProgress * 100) : null;
    return (
      <View style={[styles.banner, { backgroundColor: theme.forecastBg }]}>
        <ActivityIndicator color={theme.forecastText} />
        <Text style={[styles.bannerBody, { color: theme.forecastText }]}>
          Téléchargement d&apos;une nouvelle version{percent !== null ? ` · ${percent} %` : '…'}
        </Text>
      </View>
    );
  }
  if (!isPending) return null;

  return (
    <View style={[styles.banner, { backgroundColor: theme.accentPale }]}>
      <Text style={styles.bannerEmoji}>✨</Text>
      <View style={{ flex: 1, gap: 2 }}>
        <Text style={[styles.bannerTitle, { color: theme.accentDark }]}>Nouvelle version disponible</Text>
        <Text style={[styles.bannerBody, { color: theme.accentDark }]}>Redémarrez l&apos;app pour en profiter.</Text>
      </View>
      <Pressable
        onPress={restart}
        style={({ pressed }) => [
          styles.restartButton,
          Elevation.button,
          { backgroundColor: theme.accent, transform: [{ scale: pressed ? 0.96 : 1 }] },
        ]}
      >
        <Text style={[styles.restartText, { color: theme.onAccent }]}>Redémarrer</Text>
      </Pressable>
    </View>
  );
}

function formatUpdateDate(date: Date | undefined): string | null {
  if (!date) return null;
  return date.toLocaleDateString('fr-FR', { day: 'numeric', month: 'long', year: 'numeric' });
}

/** "Mises à jour" card for Réglages: what's running (the version built into
 * the binary, or an update and its date) and a manual "check now". */
export function AppUpdateCard() {
  const theme = useTheme();
  const { isPending, isDownloading, isChecking, currentlyRunning } = useAppUpdate();
  const [message, setMessage] = useState<{ text: string; tone: 'ok' | 'error' } | null>(null);
  const busy = isChecking || isDownloading;

  const onCheck = async () => {
    setMessage(null);
    try {
      const result = await Updates.checkForUpdateAsync();
      if (result.isAvailable || result.isRollBackToEmbedded) {
        // Progress and the "Redémarrer" banner follow from useUpdates().
        await Updates.fetchUpdateAsync();
      } else {
        setMessage({ text: 'Vous avez la dernière version 👌', tone: 'ok' });
      }
    } catch {
      setMessage({ text: 'Impossible de vérifier pour le moment. Réessayez plus tard.', tone: 'error' });
    }
  };

  const running = currentlyRunning.isEmbeddedLaunch
    ? 'Version d’origine'
    : `Mise à jour du ${formatUpdateDate(currentlyRunning.createdAt) ?? '—'}`;

  return (
    <View style={[styles.card, { backgroundColor: theme.surface }]}>
      <View style={styles.row}>
        <Text style={[styles.label, { color: theme.textSecondary }]}>En cours</Text>
        <Text style={[styles.value, { color: theme.text }]}>{running}</Text>
      </View>
      {!!currentlyRunning.runtimeVersion && (
        <View style={[styles.row, { borderTopColor: theme.line, borderTopWidth: 1 }]}>
          <Text style={[styles.label, { color: theme.textSecondary }]}>Version native</Text>
          <Text style={[styles.value, { color: theme.text }]}>{currentlyRunning.runtimeVersion}</Text>
        </View>
      )}

      {isPending ? (
        <Pressable
          onPress={restart}
          style={({ pressed }) => [styles.checkButton, { backgroundColor: theme.accent, opacity: pressed ? 0.85 : 1 }]}
        >
          <Text style={[styles.checkText, { color: theme.onAccent }]}>Redémarrer pour mettre à jour</Text>
        </Pressable>
      ) : (
        <Pressable
          onPress={onCheck}
          disabled={busy}
          style={({ pressed }) => [styles.checkButton, { backgroundColor: theme.accentPale, opacity: pressed ? 0.8 : 1 }]}
        >
          {busy ? (
            <ActivityIndicator color={theme.accentDark} />
          ) : (
            <Text style={[styles.checkText, { color: theme.accentDark }]}>Rechercher une mise à jour</Text>
          )}
        </Pressable>
      )}
      {message && (
        <Text style={[styles.message, { color: message.tone === 'ok' ? theme.positive : theme.negative }]}>
          {message.text}
        </Text>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  banner: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: Spacing.three,
    borderRadius: Radius.card,
    padding: Spacing.three,
  },
  bannerEmoji: { fontSize: 24 },
  bannerTitle: { fontSize: 15, fontWeight: '800' },
  bannerBody: { fontSize: 13, fontWeight: '600', flexShrink: 1 },
  restartButton: { borderRadius: Radius.pill, paddingHorizontal: Spacing.three, paddingVertical: 10 },
  restartText: { fontSize: 13.5, fontWeight: '800' },
  card: { ...Elevation.card, borderRadius: Radius.card, padding: Spacing.four, gap: Spacing.two },
  row: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', paddingVertical: Spacing.two, gap: Spacing.three },
  label: { fontSize: 13, fontWeight: '600' },
  value: { fontSize: 13, fontWeight: '600', flexShrink: 1, textAlign: 'right' },
  checkButton: { borderRadius: Radius.pill, paddingVertical: 12, alignItems: 'center', marginTop: Spacing.one },
  checkText: { fontSize: 14, fontWeight: '800' },
  message: { fontSize: 12.5, fontWeight: '600', textAlign: 'center' },
});
