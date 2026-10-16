import { createContext, forwardRef, useContext } from 'react';
import {
  StyleSheet,
  Text as RNText,
  TextInput as RNTextInput,
  type TextInputProps,
  type TextProps,
  type TextStyle,
} from 'react-native';
import { Typefaces } from '@/constants/theme';

function familyFor(fontWeight: TextStyle['fontWeight']): string {
  const n = fontWeight === 'bold' ? 700 : Number(fontWeight ?? 400);
  if (n >= 800) return Typefaces[800];
  if (n >= 700) return Typefaces[700];
  if (n >= 600) return Typefaces[600];
  if (n >= 500) return Typefaces[500];
  return Typefaces[400];
}

/** Maps plain RN `fontWeight` onto the matching Manrope family, so screens
 * keep their usual styles and still get the brand typeface on every
 * platform. An explicit `fontFamily` always wins. */
function brandStyle(style: TextProps['style'], nested = false): TextStyle {
  const flat = (StyleSheet.flatten(style) ?? {}) as TextStyle;
  if (flat.fontFamily) return flat;
  // A nested span with no weight of its own inherits its parent's family.
  if (nested && flat.fontWeight === undefined) return flat;
  const { fontWeight, ...rest } = flat;
  return { ...rest, fontFamily: familyFor(fontWeight) };
}

const InsideText = createContext(false);

/** Drop-in for react-native's Text, set in Manrope. */
export const Text = forwardRef<RNText, TextProps>(function Text({ style, ...rest }, ref) {
  const nested = useContext(InsideText);
  return (
    <InsideText.Provider value>
      <RNText ref={ref} style={brandStyle(style, nested)} {...rest} />
    </InsideText.Provider>
  );
});

/** Drop-in for react-native's TextInput, set in Manrope. */
export const TextInput = forwardRef<RNTextInput, TextInputProps>(function TextInput({ style, ...rest }, ref) {
  return <RNTextInput ref={ref} style={brandStyle(style)} {...rest} />;
});
