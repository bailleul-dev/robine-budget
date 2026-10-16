import * as Clipboard from 'expo-clipboard';
import { useState } from 'react';
import { ActivityIndicator, Pressable, StyleSheet, View } from 'react-native';
import { API_PUBLIC_URL } from '@/api/client';
import { useConnectedApps, useRevokeConnectedApp } from '@/api/hooks/useConnectedApps';
import type { ConnectedApp } from '@/api/types';
import { Text } from '@/components/ui/text';
import { Elevation, Radius, Spacing } from '@/constants/theme';
import { useTheme } from '@/hooks/use-theme';
import { confirmAsync } from '@/lib/confirm';

/** The server's MCP endpoint — what the user pastes into Claude (or any MCP
 * client) as a custom connector; the client then walks them through the
 * backend's own OAuth consent page. */
const MCP_URL = `${API_PUBLIC_URL}/mcp`;

function formatDate(raw: string): string {
  const d = new Date(raw);
  return Number.isNaN(d.getTime()) ? raw : d.toLocaleDateString('fr-FR', { day: 'numeric', month: 'long', year: 'numeric' });
}

/** "Applications connectées" in Réglages: the connector URL to copy, and
 * every app authorized through the OAuth consent page, each revocable on
 * the spot (GET/DELETE /api/oauth/grants). */
export function ConnectedAppsCard() {
  const theme = useTheme();
  const appsQuery = useConnectedApps();
  const revoke = useRevokeConnectedApp();
  const [copied, setCopied] = useState(false);
  const apps = appsQuery.data ?? [];

  const onCopy = async () => {
    await Clipboard.setStringAsync(MCP_URL);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const onRevoke = async (app: ConnectedApp) => {
    const confirmed = await confirmAsync(
      'Déconnecter cette application',
      `${app.client_name} n'aura plus accès à votre budget.`,
      'Déconnecter',
    );
    if (confirmed) revoke.mutate(app.id);
  };

  return (
    <View style={[styles.card, { backgroundColor: theme.surface }]}>
      <Text style={[styles.hint, { color: theme.textSecondary }]}>
        Ajoutez Lagune comme connecteur dans Claude (ou tout client MCP) avec cette adresse, puis autorisez-le.
      </Text>
      <Pressable
        onPress={onCopy}
        style={({ pressed }) => [styles.urlRow, { backgroundColor: theme.surfaceSoft, opacity: pressed ? 0.8 : 1 }]}
      >
        <Text style={[styles.url, { color: theme.text }]} numberOfLines={1}>
          {MCP_URL}
        </Text>
        <Text style={[styles.copy, { color: theme.accentDark }]}>{copied ? 'Copié' : 'Copier'}</Text>
      </Pressable>

      {appsQuery.isLoading && <ActivityIndicator color={theme.accent} style={styles.spacer} />}
      {appsQuery.isError && (
        <Text style={[styles.spacer, { color: theme.negative }]}>Impossible de charger les applications.</Text>
      )}
      {!appsQuery.isLoading && !appsQuery.isError && apps.length === 0 && (
        <Text style={[styles.empty, { color: theme.subtle }]}>Aucune application connectée pour l&apos;instant.</Text>
      )}

      {apps.map((app, i) => (
        <View key={app.id} style={[styles.appRow, i === 0 ? styles.firstRow : { borderTopColor: theme.line, borderTopWidth: 1 }]}>
          <View style={[styles.avatar, { backgroundColor: theme.forecastBg }]}>
            <Text style={[styles.avatarText, { color: theme.forecastText }]}>
              {app.client_name.trim().charAt(0).toUpperCase() || '?'}
            </Text>
          </View>
          <View style={styles.appText}>
            <Text style={[styles.appName, { color: theme.text }]} numberOfLines={1}>
              {app.client_name}
            </Text>
            <Text style={[styles.appMeta, { color: theme.textSecondary }]} numberOfLines={1}>
              Autorisée le {formatDate(app.created_at)}
            </Text>
          </View>
          <Pressable
            onPress={() => onRevoke(app)}
            disabled={revoke.isPending}
            style={({ pressed }) => [styles.revoke, { backgroundColor: theme.negativeBg, opacity: pressed ? 0.8 : 1 }]}
          >
            <Text style={[styles.revokeText, { color: theme.negative }]}>Déconnecter</Text>
          </Pressable>
        </View>
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  card: { ...Elevation.card, borderRadius: Radius.card, padding: Spacing.four, gap: Spacing.three },
  hint: { fontSize: 13, fontWeight: '500', lineHeight: 19 },
  urlRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: Spacing.two,
    borderRadius: Radius.control,
    paddingHorizontal: Spacing.three,
    paddingVertical: 12,
  },
  url: { flex: 1, fontSize: 13, fontWeight: '600' },
  copy: { fontSize: 13, fontWeight: '800' },
  spacer: { marginTop: Spacing.two },
  empty: { fontSize: 13, fontWeight: '500' },
  appRow: { flexDirection: 'row', alignItems: 'center', gap: Spacing.three, paddingTop: Spacing.three },
  firstRow: { paddingTop: Spacing.one },
  avatar: { width: 36, height: 36, borderRadius: Radius.pill, alignItems: 'center', justifyContent: 'center' },
  avatarText: { fontSize: 15, fontWeight: '800' },
  appText: { flex: 1, gap: 2 },
  appName: { fontSize: 15, fontWeight: '700' },
  appMeta: { fontSize: 12.5, fontWeight: '500' },
  revoke: { borderRadius: Radius.pill, paddingHorizontal: 14, paddingVertical: 8 },
  revokeText: { fontSize: 13, fontWeight: '800' },
});
