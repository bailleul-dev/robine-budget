import { useState } from 'react';
import { View } from 'react-native';
import Svg, { Circle, Defs, LinearGradient, Path, Stop } from 'react-native-svg';
import type { BalancePoint } from '@/domain/dashboard';
import { useTheme } from '@/hooks/use-theme';

type XY = [number, number];

/** Smooth cubic path through points (Catmull-Rom converted to Béziers) —
 * a balance curve should read as a wave, not a zig-zag. */
function smoothPath(pts: XY[]): string {
  if (pts.length === 0) return '';
  let d = `M${pts[0][0]},${pts[0][1]}`;
  for (let i = 0; i < pts.length - 1; i++) {
    const p0 = pts[i - 1] ?? pts[i];
    const p1 = pts[i];
    const p2 = pts[i + 1];
    const p3 = pts[i + 2] ?? p2;
    const c1: XY = [p1[0] + (p2[0] - p0[0]) / 6, p1[1] + (p2[1] - p0[1]) / 6];
    const c2: XY = [p2[0] - (p3[0] - p1[0]) / 6, p2[1] - (p3[1] - p1[1]) / 6];
    d += ` C${c1[0]},${c1[1]} ${c2[0]},${c2[1]} ${p2[0]},${p2[1]}`;
  }
  return d;
}

/** The hero card's balance "wave": past days as a lagoon line with a soft
 * fill beneath, the forecast continuing in dashed lilac, and a dot on
 * today. Decorative summary only — the scrubbable chart lives on
 * graphs.tsx. */
export function WaveSparkline({ points, height = 72 }: { points: BalancePoint[]; height?: number }) {
  const theme = useTheme();
  const [width, setWidth] = useState(0);
  const pad = 6;

  let content = null;
  if (width > 0 && points.length > 1) {
    const values = points.map((p) => p.balance);
    const min = Math.min(...values);
    const max = Math.max(...values);
    const span = max - min || 1;
    const xy = points.map((p, i): XY => [
      (i / (points.length - 1)) * width,
      pad + (1 - (p.balance - min) / span) * (height - pad * 2),
    ]);
    const todayIndex = Math.max(0, points.findIndex((p) => p.isForecast) - 1);
    const past = xy.slice(0, todayIndex + 1);
    const future = xy.slice(todayIndex);
    const pastLine = smoothPath(past);
    const today = xy[todayIndex];

    content = (
      <Svg width={width} height={height}>
        <Defs>
          <LinearGradient id="wave-fill" x1="0" y1="0" x2="0" y2="1">
            <Stop offset="0" stopColor={theme.accent} stopOpacity={0.28} />
            <Stop offset="1" stopColor={theme.accent} stopOpacity={0} />
          </LinearGradient>
        </Defs>
        {past.length > 1 && <Path d={`${pastLine} L${today[0]},${height} L0,${height} Z`} fill="url(#wave-fill)" />}
        {past.length > 1 && (
          <Path d={pastLine} stroke={theme.accent} strokeWidth={3} fill="none" strokeLinecap="round" />
        )}
        {future.length > 1 && (
          <Path
            d={smoothPath(future)}
            stroke={theme.lilac}
            strokeWidth={3}
            strokeDasharray="1 7"
            strokeLinecap="round"
            fill="none"
          />
        )}
        <Circle cx={today[0]} cy={today[1]} r={7} fill={theme.surface} />
        <Circle cx={today[0]} cy={today[1]} r={4.5} fill={theme.accent} />
      </Svg>
    );
  }

  return (
    <View style={{ height }} onLayout={(e) => setWidth(e.nativeEvent.layout.width)}>
      {content}
    </View>
  );
}
