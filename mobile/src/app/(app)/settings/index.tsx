import Constants from "expo-constants";
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, View } from "react-native";
import { Text, TextInput } from "@/components/ui/text";
import { useVersion } from "@/api/hooks/useVersion";
import { useAuth } from "@/auth/AuthContext";
import { AppUpdateBanner, AppUpdateCard, appUpdatesSupported } from "@/components/AppUpdate";
import { useBankSyncStatus } from "@/api/hooks/useBankSync";
import { BankSyncCard } from "@/components/BankSyncCard";
import { EmailVerificationNotice } from "@/components/EmailVerificationNotice";
import { ConnectedAppsCard } from "@/components/ConnectedAppsCard";
import { DeleteAccountSection } from "@/components/DeleteAccountSection";
import { NavIcon, NavIcons, type IconName } from "@/components/nav-icon";
import { Elevation, Radius, Spacing } from "@/constants/theme";
import { useScreenTopClearance } from "@/hooks/use-screen-top-clearance";
import { useTabBarClearance } from "@/hooks/use-tab-bar-clearance";
import { useTheme } from "@/hooks/use-theme";
import { useThemePreference, type ThemePreference } from "@/theme/ThemeProvider";
import * as Clipboard from "expo-clipboard";
import { useState } from "react";
import { ApiError, WEB_APP_URL } from "@/api/client";
import {
  useCollaborators,
  useCreateCollaboratorInvite,
  useRevokeCollaborator,
} from "@/api/hooks/useCollaborators";
import type { Collaborator, CollaboratorStatus } from "@/api/types";
import { confirmAsync } from "@/lib/confirm";

const STATUS_LABEL: Record<CollaboratorStatus, string> = {
  pending: "En attente",
  accepted: "Accepté",
  revoked: "Révoqué",
};

function shareUrl(token: string): string {
  return `${WEB_APP_URL}/invite/${token}`;
}

/** Splits a free-typed "email, email; email" field into individual
 * addresses — lets the user invite several people in one submit without
 * the backend needing to know about batches (each is still its own,
 * separately revocable invite). Ported from web's SharingPage. */
function splitEmails(raw: string): string[] {
  return raw
    .split(/[,;\s]+/)
    .map((e) => e.trim())
    .filter(Boolean);
}

function formatBuildDate(raw: string): string {
  const d = new Date(raw);
  return Number.isNaN(d.getTime()) ? raw : d.toLocaleString("fr-FR");
}

const THEME_OPTIONS: {
  value: ThemePreference;
  label: string;
  icon: IconName;
}[] = [
  { value: "light", label: "Clair", icon: NavIcons.themeLight },
  { value: "dark", label: "Sombre", icon: NavIcons.themeDark },
  { value: "system", label: "Auto", icon: NavIcons.themeAuto },
];

/** Three-way segmented control — a persistent, always-visible row rather
 * than a modal picker (see OptionPicker), since there are only three
 * options and this is exactly the kind of choice iOS/Android's own Settings
 * apps present the same way. */
function ThemeSwitcher() {
  const theme = useTheme();
  const { preference, setPreference } = useThemePreference();

  return (
    <View
      style={[
        styles.themeSwitcher,
        { backgroundColor: theme.surfaceSoft, borderColor: theme.line },
      ]}
    >
      {THEME_OPTIONS.map((option) => {
        const selected = option.value === preference;
        return (
          <Pressable
            key={option.value}
            onPress={() => setPreference(option.value)}
            style={[
              styles.themeOption,
              selected && { backgroundColor: theme.accent },
            ]}
          >
            <NavIcon
              name={option.icon}
              color={selected ? "#fff" : theme.textSecondary}
              size={16}
            />
            <Text
              style={[
                styles.themeOptionText,
                { color: selected ? "#fff" : theme.textSecondary },
              ]}
            >
              {option.label}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}

/** Minimal first pass: account identity (nothing editable yet — changing
 * display name/email/password isn't wired up on mobile or web), sign-out
 * (moved here from the drawer footer, which now only shows the email), and
 * the running server's build info, same public /version endpoint as the
 * web app's VersionScreen. */
export default function SettingsScreen() {
  const theme = useTheme();
  const tabBarClearance = useTabBarClearance();
  const topClearance = useScreenTopClearance();
  const { user, logout, logoutEverywhere } = useAuth();
  // Bank sync is closed to most accounts (allow-list server-side): hide the
  // whole section unless the server says it's available. Until the status
  // loads it stays hidden rather than flashing in.
  const bankSyncAvailable = useBankSyncStatus().data?.available === true;
  const versionQuery = useVersion();
  const version = versionQuery.data;
  const collaboratorsQuery = useCollaborators();
  const createInvite = useCreateCollaboratorInvite();
  const revokeCollaborator = useRevokeCollaborator();
  const [emailInput, setEmailInput] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const collaborators = collaboratorsQuery.data ?? [];

  const onCreate = async () => {
    setError(null);
    const emails = splitEmails(emailInput);
    if (emails.length === 0) return;
    const failures: string[] = [];
    for (const email of emails) {
      try {
        await createInvite.mutateAsync(email);
      } catch (err) {
        failures.push(
          `${email} : ${err instanceof ApiError ? err.message : "Une erreur est survenue."}`,
        );
      }
    }
    if (failures.length === 0) {
      setEmailInput("");
    } else {
      setError(failures.join(" — "));
    }
  };

  const onCopy = async (c: Collaborator) => {
    await Clipboard.setStringAsync(shareUrl(c.token));
    setCopiedId(c.id);
    setTimeout(() => setCopiedId((id) => (id === c.id ? null : id)), 2000);
  };

  const onRevoke = async (c: Collaborator) => {
    const confirmed = await confirmAsync(
      "Révoquer ce lien",
      c.collaborator_name
        ? `${c.collaborator_name} n'aura plus accès à vos comptes.`
        : "Ce lien d'invitation ne pourra plus être utilisé.",
      "Révoquer",
    );
    if (!confirmed) return;
    revokeCollaborator.mutate(c.id);
  };

  return (
    <ScrollView
      style={[styles.container, { backgroundColor: theme.background }]}
      contentContainerStyle={[styles.content, { paddingTop: Spacing.four + topClearance, paddingBottom: Spacing.six + tabBarClearance }]}
    >
      <AppUpdateBanner />

      {user && (
        <View>
          <Text style={[styles.sectionTitle, { color: theme.text }]}>
            Compte
          </Text>
          <View
            style={[
              styles.card,
              { backgroundColor: theme.surface, borderColor: theme.line },
            ]}
          >
            <Text style={[styles.name, { color: theme.text }]}>
              {user.display_name}
            </Text>
            {user.email && (
              <Text style={[styles.email, { color: theme.textSecondary }]}>
                {user.email}
              </Text>
            )}
            <Pressable
              onPress={() => logout()}
              style={({ pressed }) => [
                styles.logoutRow,
                { borderTopColor: theme.line, opacity: pressed ? 0.7 : 1 },
              ]}
            >
              <NavIcon
                name={NavIcons.logout}
                color={theme.accentDark}
                size={16}
              />
              <Text style={[styles.logoutText, { color: theme.accentDark }]}>
                Se déconnecter
              </Text>
            </Pressable>
            <Pressable
              onPress={async () => {
                const confirmed = await confirmAsync(
                  "Se déconnecter partout",
                  "Tous vos appareils, y compris celui-ci, seront déconnectés. Vous devrez vous reconnecter.",
                  "Se déconnecter partout",
                );
                if (confirmed) await logoutEverywhere().catch(() => undefined);
              }}
              style={({ pressed }) => [
                styles.logoutRow,
                { borderTopColor: theme.line, opacity: pressed ? 0.7 : 1 },
              ]}
            >
              <NavIcon name={NavIcons.logout} color={theme.negative} size={16} />
              <Text style={[styles.logoutText, { color: theme.negative }]}>
                Se déconnecter de tous les appareils
              </Text>
            </Pressable>
          </View>
          <EmailVerificationNotice />
        </View>
      )}

      <View>
        <Text style={[styles.sectionTitle, { color: theme.text }]}>
          Apparence
        </Text>
        <ThemeSwitcher />
      </View>

      {bankSyncAvailable && (
        <View>
          <Text style={[styles.sectionTitle, { color: theme.text }]}>
            Synchronisation bancaire
          </Text>
          <BankSyncCard />
        </View>
      )}

      <View>
        <Text style={[styles.sectionTitle, { color: theme.text }]}>
          Applications connectées
        </Text>
        <ConnectedAppsCard />
      </View>

      {appUpdatesSupported && (
        <View>
          <Text style={[styles.sectionTitle, { color: theme.text }]}>
            Mises à jour
          </Text>
          <AppUpdateCard />
        </View>
      )}

      <View>
        <Text style={[styles.sectionTitle, { color: theme.text }]}>
          À propos
        </Text>
        <View
          style={[
            styles.card,
            { backgroundColor: theme.surface, borderColor: theme.line },
          ]}
        >
          {versionQuery.isLoading && <ActivityIndicator color={theme.accent} />}
          {versionQuery.isError && (
            <Text style={{ color: theme.negative }}>
              Impossible de charger la version.
            </Text>
          )}
          {version && (
            <>
              <View style={styles.row}>
                <Text style={[styles.rowLabel, { color: theme.textSecondary }]}>
                  Version app
                </Text>
                <Text style={[styles.rowValue, { color: theme.text }]}>
                  {Constants.expoConfig?.version ?? "—"}
                </Text>
              </View>
              <View
                style={[
                  styles.row,
                  { borderTopColor: theme.line, borderTopWidth: 1 },
                ]}
              >
                <Text style={[styles.rowLabel, { color: theme.textSecondary }]}>
                  Branche serveur
                </Text>
                <Text style={[styles.rowValue, { color: theme.text }]}>
                  {version.branch}
                </Text>
              </View>
              <View
                style={[
                  styles.row,
                  { borderTopColor: theme.line, borderTopWidth: 1 },
                ]}
              >
                <Text style={[styles.rowLabel, { color: theme.textSecondary }]}>
                  Commit
                </Text>
                <Text
                  style={[styles.rowValue, { color: theme.text }]}
                  numberOfLines={1}
                >
                  {version.commit.slice(0, 12)}
                </Text>
              </View>
              <View
                style={[
                  styles.row,
                  { borderTopColor: theme.line, borderTopWidth: 1 },
                ]}
              >
                <Text style={[styles.rowLabel, { color: theme.textSecondary }]}>
                  Construit le
                </Text>
                <Text style={[styles.rowValue, { color: theme.text }]}>
                  {formatBuildDate(version.build_date)}
                </Text>
              </View>
            </>
          )}
        </View>
      </View>

      <Text style={[styles.subheading, { color: theme.textSecondary }]}>
        Donnez à quelqu&apos;un d&apos;autre accès à vos comptes via un lien
        nominatif — seule la personne connectée avec cette adresse e-mail exacte
        peut l&apos;utiliser.
      </Text>

      <View
        style={[
          styles.card,
          { backgroundColor: theme.surface, borderColor: theme.line },
        ]}
      >
        <Text style={[styles.label, { color: theme.textSecondary }]}>
          Adresse e-mail
        </Text>
        <View style={styles.inviteRow}>
          <TextInput
            value={emailInput}
            onChangeText={setEmailInput}
            autoCapitalize="none"
            keyboardType="email-address"
            placeholder="nom@exemple.com"
            style={[
              styles.input,
              { borderColor: theme.line, color: theme.text },
            ]}
            placeholderTextColor={theme.subtle}
          />
          <Pressable
            onPress={onCreate}
            disabled={createInvite.isPending || !emailInput.trim()}
            style={[
              styles.submit,
              {
                backgroundColor: theme.accent,
                opacity: createInvite.isPending || !emailInput.trim() ? 0.6 : 1,
              },
            ]}
          >
            <Text style={styles.submitText}>
              {createInvite.isPending ? "Création…" : "Créer le lien"}
            </Text>
          </Pressable>
        </View>
        {error && (
          <Text
            style={{
              color: theme.negative,
              fontSize: 12.5,
              marginTop: Spacing.two,
            }}
          >
            {error}
          </Text>
        )}
      </View>

      {collaboratorsQuery.isLoading && (
        <ActivityIndicator color={theme.accent} />
      )}
      {collaboratorsQuery.isError && (
        <Text style={{ color: theme.negative }}>
          Impossible de charger les partages.
        </Text>
      )}

      <View style={styles.list}>
        {collaborators.map((c) => (
          <View
            key={c.id}
            style={[
              styles.card,
              { backgroundColor: theme.surface, borderColor: theme.line },
            ]}
          >
            <View style={styles.rowTop}>
              <Text
                style={[styles.person, { color: theme.text }]}
                numberOfLines={1}
              >
                {c.collaborator_name ?? c.invited_email}
              </Text>
              <View
                style={[
                  styles.tag,
                  {
                    backgroundColor:
                      c.status === "accepted"
                        ? theme.positiveBg
                        : c.status === "pending"
                          ? theme.forecastBg
                          : theme.surfaceSoft,
                  },
                ]}
              >
                <Text
                  style={{
                    color:
                      c.status === "accepted"
                        ? theme.positive
                        : c.status === "pending"
                          ? theme.forecastText
                          : theme.textSecondary,
                    fontSize: 10,
                    fontWeight: "700",
                    textTransform: "uppercase",
                  }}
                >
                  {STATUS_LABEL[c.status]}
                </Text>
              </View>
            </View>

            {c.status !== "revoked" ? (
              <View style={styles.actionsRow}>
                <Pressable onPress={() => onCopy(c)} hitSlop={8}>
                  <Text
                    style={[styles.actionText, { color: theme.accentDark }]}
                  >
                    {copiedId === c.id ? "Lien copié" : "Copier le lien"}
                  </Text>
                </Pressable>
                <Pressable onPress={() => onRevoke(c)} hitSlop={8}>
                  <Text style={[styles.actionText, { color: theme.negative }]}>
                    Révoquer
                  </Text>
                </Pressable>
              </View>
            ) : (
              <Text style={[styles.disabledText, { color: theme.subtle }]}>
                Lien désactivé
              </Text>
            )}
          </View>
        ))}
        {!collaboratorsQuery.isLoading && collaborators.length === 0 && (
          <Text style={{ color: theme.textSecondary }}>
            Aucun partage pour l&apos;instant.
          </Text>
        )}
      </View>

      {user && <DeleteAccountSection />}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  content: {
    padding: Spacing.four,
    gap: Spacing.four,
    paddingBottom: Spacing.six,
  },
  sectionTitle: { fontSize: 17, fontWeight: "800", letterSpacing: -0.3, marginBottom: Spacing.two, marginLeft: Spacing.one },
  themeSwitcher: {
    flexDirection: "row",
    borderWidth: 1,
    borderRadius: Radius.pill,
    padding: 4,
    gap: 4,
  },
  themeOption: {
    flex: 1,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: 6,
    paddingVertical: Spacing.two,
    borderRadius: Radius.pill,
  },
  themeOptionText: { fontSize: 13, fontWeight: "700" },
  card: { ...Elevation.card, borderRadius: Radius.card, padding: Spacing.four },
  name: { fontSize: 16, fontWeight: "700" },
  email: { fontSize: 13, marginTop: 2 },
  logoutRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: Spacing.two,
    marginTop: Spacing.three,
    paddingTop: Spacing.three,
    borderTopWidth: 1,
  },
  logoutText: { fontSize: 13, fontWeight: "700" },
  row: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingVertical: Spacing.two,
    gap: Spacing.three,
  },
  rowLabel: { fontSize: 13, fontWeight: "600" },
  rowValue: { fontSize: 13, flexShrink: 1, textAlign: "right" },
  subheading: { fontSize: 12.5, lineHeight: 17 },
  label: { fontSize: 11, fontWeight: "700" },
  inviteRow: {
    flexDirection: "row",
    gap: Spacing.two,
    marginTop: 4,
    alignItems: "center",
  },
  input: {
    flex: 1,
    borderWidth: 1,
    borderRadius: Radius.control,
    paddingHorizontal: Spacing.three,
    paddingVertical: 12,
    fontSize: 14,
  },
  submit: {
    ...Elevation.button,
    borderRadius: Radius.pill,
    paddingHorizontal: Spacing.four,
    paddingVertical: 12,
  },
  submitText: { color: "#fff", fontWeight: "700", fontSize: 13 },
  list: { gap: Spacing.three },
  rowTop: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: Spacing.two,
  },
  person: { fontSize: 14.5, fontWeight: "700", flexShrink: 1 },
  tag: {
    borderRadius: 999,
    paddingHorizontal: Spacing.two,
    paddingVertical: 4,
  },
  actionsRow: {
    flexDirection: "row",
    gap: Spacing.four,
    marginTop: Spacing.two,
  },
  actionText: { fontSize: 12.5, fontWeight: "700" },
  disabledText: { fontSize: 12.5, marginTop: Spacing.two },
});
