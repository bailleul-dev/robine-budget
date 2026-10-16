import type { Meta, StoryObj } from '@storybook/react-native-web-vite';
import { StyleSheet, View } from 'react-native';
import { Text } from '@/components/ui/text';
import { Logo } from '@/components/ui/logo';
import { Colors, Radius, Spacing } from '@/constants/theme';

/** "Lagune" — money as calm water: misty neutrals, a lagoon teal brand
 * hue, lilac for everything still ahead (forecasts, planned items) and a
 * soft coral for spending instead of alarm red. See constants/theme.ts for
 * the rationale. This page is a living reference for those tokens — change
 * one there and this updates with it. */
function Swatch({ name, hex }: { name: string; hex: string }) {
  return (
    <View style={styles.swatchRow}>
      <View style={[styles.swatch, { backgroundColor: hex, borderColor: 'rgba(0,0,0,0.08)' }]} />
      <View>
        <Text style={styles.swatchName}>{name}</Text>
        <Text style={styles.swatchHex}>{hex}</Text>
      </View>
    </View>
  );
}

function Palette({ title, tokens }: { title: string; tokens: Record<string, string> }) {
  return (
    <View style={styles.paletteBlock}>
      <Text style={styles.paletteTitle}>{title}</Text>
      <View style={styles.swatchGrid}>
        {Object.entries(tokens).map(([name, hex]) => (
          <Swatch key={name} name={name} hex={hex} />
        ))}
      </View>
    </View>
  );
}

function SpacingScale() {
  return (
    <View style={styles.paletteBlock}>
      <Text style={styles.paletteTitle}>Spacing</Text>
      {Object.entries(Spacing).map(([name, value]) => (
        <View key={name} style={styles.spacingRow}>
          <Text style={styles.spacingLabel}>
            {name} — {value}px
          </Text>
          <View style={[styles.spacingBar, { width: value * 2, backgroundColor: Colors.light.accent }]} />
        </View>
      ))}
    </View>
  );
}

function DesignTokensPage() {
  return (
    <View style={{ gap: Spacing.five }}>
      <View>
        <Logo size={44} />
        <Text style={styles.pageSubtitle}>
          Design tokens — votre argent, tout en douceur. Lagon pour la marque, lilas pour ce qui est à venir, corail
          doux pour les dépenses, et des formes arrondies comme des galets.
        </Text>
      </View>
      <View style={styles.sideBySide}>
        <Palette title="Light" tokens={Colors.light} />
        <Palette title="Dark" tokens={Colors.dark} />
      </View>
      <SpacingScale />
    </View>
  );
}

const meta = {
  title: 'Design System/Tokens',
  component: DesignTokensPage,
} satisfies Meta<typeof DesignTokensPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Palette_: Story = {
  name: 'Colors & Spacing',
};

const styles = StyleSheet.create({
  pageSubtitle: { fontSize: 14, color: Colors.light.textSecondary, marginTop: Spacing.three, maxWidth: 560, lineHeight: 20 },
  sideBySide: { flexDirection: 'row', flexWrap: 'wrap', gap: Spacing.five },
  paletteBlock: { gap: Spacing.two, minWidth: 260 },
  paletteTitle: { fontSize: 13, fontWeight: '800', textTransform: 'uppercase', letterSpacing: 0.5, color: Colors.light.textSecondary },
  swatchGrid: { gap: Spacing.two },
  swatchRow: { flexDirection: 'row', alignItems: 'center', gap: Spacing.three },
  swatch: { width: 44, height: 44, borderRadius: Radius.control, borderWidth: 1 },
  swatchName: { fontSize: 13, fontWeight: '700', color: Colors.light.text },
  swatchHex: { fontSize: 11, color: Colors.light.subtle, fontVariant: ['tabular-nums'] },
  spacingRow: { flexDirection: 'row', alignItems: 'center', gap: Spacing.three, marginTop: 4 },
  spacingLabel: { fontSize: 12.5, width: 110, color: Colors.light.text },
  spacingBar: { height: 10, borderRadius: Radius.pill },
});
