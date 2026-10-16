import { Pressable, StyleSheet, View } from 'react-native';
import { NavIcon, NavIcons } from '@/components/nav-icon';
import { Text } from '@/components/ui/text';
import { Elevation, Radius, Spacing } from '@/constants/theme';
import { PERIOD_PRESETS, periodRange, type Period } from '@/domain/period';
import { useTheme } from '@/hooks/use-theme';

/** Two rows: how long a window (Mois · 3 mois · … · 2 ans, as pills), then
 * which one (‹ septembre 2026 ›). The arrows step a whole window at a time
 * for every preset, so "a precise month" and "last year's second half" are
 * the same gesture; the future is never reachable, and "Aujourd'hui" jumps
 * back once you've wandered off. */
export function PeriodPicker({ value, onChange }: { value: Period; onChange: (period: Period) => void }) {
  const theme = useTheme();
  const range = periodRange(value);

  return (
    <View style={styles.container}>
      <View style={styles.presets}>
        {PERIOD_PRESETS.map((preset) => {
          const selected = preset.value === value.preset;
          return (
            <Pressable
              key={preset.value}
              accessibilityRole="button"
              accessibilityState={{ selected }}
              onPress={() => onChange({ preset: preset.value, offset: 0 })}
              style={({ pressed }) => [
                styles.preset,
                selected ? [styles.presetSelected, { backgroundColor: theme.accent }] : { backgroundColor: theme.surfaceSoft },
                { opacity: pressed ? 0.8 : 1 },
              ]}
            >
              <Text style={[styles.presetText, { color: selected ? theme.onAccent : theme.textSecondary }]}>{preset.label}</Text>
            </Pressable>
          );
        })}
      </View>

      <View style={[styles.stepper, { backgroundColor: theme.surface }]}>
        <Pressable
          accessibilityRole="button"
          accessibilityLabel="Période précédente"
          onPress={() => onChange({ ...value, offset: value.offset - 1 })}
          hitSlop={8}
          style={({ pressed }) => [styles.arrow, { backgroundColor: theme.surfaceSoft, opacity: pressed ? 0.7 : 1 }]}
        >
          <NavIcon name={NavIcons.previous} color={theme.accentDark} size={16} />
        </Pressable>

        <View style={styles.labelBox}>
          <Text style={[styles.label, { color: theme.text }]} numberOfLines={1}>
            {range.label}
          </Text>
          {!range.isCurrent && (
            <Pressable accessibilityRole="button" onPress={() => onChange({ ...value, offset: 0 })} hitSlop={6}>
              <Text style={[styles.today, { color: theme.accentDark }]}>Aujourd&apos;hui</Text>
            </Pressable>
          )}
        </View>

        <Pressable
          accessibilityRole="button"
          accessibilityLabel="Période suivante"
          disabled={range.isCurrent}
          onPress={() => onChange({ ...value, offset: value.offset + 1 })}
          hitSlop={8}
          style={({ pressed }) => [
            styles.arrow,
            { backgroundColor: theme.surfaceSoft, opacity: range.isCurrent ? 0.35 : pressed ? 0.7 : 1 },
          ]}
        >
          <NavIcon name={NavIcons.next} color={theme.accentDark} size={16} />
        </Pressable>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { gap: Spacing.three },
  presets: { flexDirection: 'row', gap: 6 },
  preset: { flex: 1, alignItems: 'center', borderRadius: Radius.pill, paddingHorizontal: 4, paddingVertical: Spacing.two },
  presetSelected: { ...Elevation.button },
  presetText: { fontSize: 13, fontWeight: '700' },
  stepper: {
    ...Elevation.card,
    flexDirection: 'row',
    alignItems: 'center',
    gap: Spacing.two,
    borderRadius: Radius.pill,
    padding: 6,
  },
  arrow: { width: 36, height: 36, borderRadius: Radius.pill, alignItems: 'center', justifyContent: 'center' },
  labelBox: { flex: 1, alignItems: 'center', gap: 1 },
  label: { fontSize: 15, fontWeight: '800', letterSpacing: -0.2, textTransform: 'capitalize' },
  today: { fontSize: 11.5, fontWeight: '700' },
});
