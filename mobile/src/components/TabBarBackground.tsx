import { StyleSheet, View } from 'react-native';
import { BlurView } from 'expo-blur';
import { GlassView, isLiquidGlassAvailable } from 'expo-glass-effect';
import { useTheme } from '@/hooks/use-theme';
import { useResolvedScheme } from '@/theme/ThemeProvider';

/** The floating pill behind the mobile tab bar (see MobileTabs in
 * (app)/_layout.tsx for its geometry).
 *
 * On iOS 26 it is real Liquid Glass, the system's own material. Everywhere
 * else — Android, web, older iOS — expo-blur's BlurView gives a backdrop
 * blur that renders in the PWA too, unlike GlassView, which is a native
 * iOS 26 API and silently no-ops elsewhere. */
export function TabBarBackground({ top, bottom, side, radius }: { top: number; bottom: number; side: number; radius: number }) {
  const theme = useTheme();
  const scheme = useResolvedScheme();
  const frame = { top, bottom, left: side, right: side, borderRadius: radius };

  if (isLiquidGlassAvailable()) {
    // "clear" rather than "regular": barely tinted, so the content keeps
    // showing through the bar.
    return <GlassView glassEffectStyle="clear" style={[styles.pill, frame]} />;
  }

  return (
    <View style={[styles.pill, styles.clipped, frame, { borderColor: theme.line }]}>
      <BlurView intensity={60} tint={scheme === 'dark' ? 'dark' : 'light'} style={StyleSheet.absoluteFill} />
    </View>
  );
}

const styles = StyleSheet.create({
  pill: { position: 'absolute' },
  clipped: { overflow: 'hidden', borderWidth: StyleSheet.hairlineWidth },
});
