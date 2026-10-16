import { View } from 'react-native';
import Svg, { Circle, Defs, G, LinearGradient, Path, RadialGradient, Rect, Stop } from 'react-native-svg';
import { Text } from '@/components/ui/text';
import { useTheme } from '@/hooks/use-theme';

/** The Lagune mark: a lowercase "l" whose foot rolls out into a small
 * wave, with a sun (or a coin, or a bubble) floating above it, on a
 * lagoon-to-lilac pebble. Same geometry as assets/brand/mark.svg, which
 * every app icon and favicon is rendered from. */
export function LogoMark({ size = 32 }: { size?: number }) {
  return (
    <Svg width={size} height={size} viewBox="0 0 64 64">
      <Defs>
        <LinearGradient id="lagune-g" x1="0.1" y1="0" x2="0.9" y2="1">
          <Stop offset="0" stopColor="#4fd8c4" />
          <Stop offset="0.5" stopColor="#23a898" />
          <Stop offset="1" stopColor="#9d8ff0" />
        </LinearGradient>
        <RadialGradient id="lagune-h" cx="0.22" cy="0.15" r="0.75">
          <Stop offset="0" stopColor="#fff" stopOpacity={0.4} />
          <Stop offset="1" stopColor="#fff" stopOpacity={0} />
        </RadialGradient>
      </Defs>
      <Rect x={2} y={2} width={60} height={60} rx={19} fill="url(#lagune-g)" />
      <Rect x={2} y={2} width={60} height={60} rx={19} fill="url(#lagune-h)" />
      <G transform="translate(-4 1.5)">
        <Path
          d="M22 13 V38 C22 45 26 48 31 48 C36 48 38 43 42.5 43 C47 43 48 48 52 48"
          fill="none"
          stroke="#fff"
          strokeWidth={7.5}
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <Circle cx={45} cy={20.5} r={4.6} fill="#fff" fillOpacity={0.9} />
      </G>
    </Svg>
  );
}

/** Mark + wordmark lockup. */
export function Logo({ size = 32 }: { size?: number }) {
  const theme = useTheme();
  return (
    <View style={{ flexDirection: 'row', alignItems: 'center', gap: size * 0.3 }}>
      <LogoMark size={size} />
      <Text style={{ fontSize: size * 0.66, fontWeight: '800', letterSpacing: -0.4, color: theme.text }}>lagune</Text>
    </View>
  );
}
