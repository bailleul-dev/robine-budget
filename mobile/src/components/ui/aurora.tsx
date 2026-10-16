import { StyleSheet, View } from 'react-native';
import Svg, { Circle, Defs, RadialGradient, Stop } from 'react-native-svg';
import { useResolvedScheme } from '@/theme/ThemeProvider';

type Blob = { id: string; cx: string; cy: string; r: string; color: string; opacity: number };

const BLOBS: Record<'light' | 'dark', Blob[]> = {
  light: [
    { id: 'a', cx: '12%', cy: '5%', r: '45%', color: '#5fdcc9', opacity: 0.55 },
    { id: 'b', cx: '88%', cy: '10%', r: '42%', color: '#b3a6ff', opacity: 0.5 },
    { id: 'c', cx: '55%', cy: '40%', r: '30%', color: '#9fe6f0', opacity: 0.35 },
  ],
  dark: [
    { id: 'a', cx: '12%', cy: '5%', r: '45%', color: '#1f9e8e', opacity: 0.45 },
    { id: 'b', cx: '88%', cy: '10%', r: '42%', color: '#6b5ad3', opacity: 0.4 },
    { id: 'c', cx: '55%', cy: '40%', r: '30%', color: '#1d6f86', opacity: 0.3 },
  ],
};

/** The soft "aurora" wash that sits behind hero areas: a few big radial
 * gradients fading to transparent, so it reads as blurred light on water
 * rather than as shapes. Pure SVG gradients (no blur filter), so it looks
 * the same on iOS, Android and web. Place it first inside a
 * position:relative container; it fills it and ignores touches. `bleedTop`
 * pulls it above that container's own top padding — a scroll view that
 * pads its content for the status bar still gets the wash behind it (see
 * useScreenTopClearance). */
export function Aurora({ height, bleedTop = 0 }: { height?: number; bleedTop?: number }) {
  const scheme = useResolvedScheme();
  return (
    <View
      pointerEvents="none"
      style={[
        StyleSheet.absoluteFill,
        height !== undefined && { height: height + bleedTop, bottom: undefined },
        bleedTop > 0 && { top: -bleedTop },
      ]}
    >
      <Svg width="100%" height="100%">
        <Defs>
          {BLOBS[scheme].map((b) => (
            <RadialGradient key={b.id} id={`aurora-${b.id}`} cx="50%" cy="50%" r="50%">
              <Stop offset="0" stopColor={b.color} stopOpacity={b.opacity} />
              <Stop offset="0.45" stopColor={b.color} stopOpacity={b.opacity * 0.45} />
              <Stop offset="0.75" stopColor={b.color} stopOpacity={b.opacity * 0.12} />
              <Stop offset="1" stopColor={b.color} stopOpacity={0} />
            </RadialGradient>
          ))}
        </Defs>
        {BLOBS[scheme].map((b) => (
          <Circle key={b.id} cx={b.cx} cy={b.cy} r={b.r} fill={`url(#aurora-${b.id})`} />
        ))}
      </Svg>
    </View>
  );
}
