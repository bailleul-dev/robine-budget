import { Alert, Platform } from 'react-native';

// Alert.alert() is a silent no-op on react-native-web (see
// node_modules/react-native-web/dist/exports/Alert) — this app runs there
// too (`expo start --web`), so any destructive action needs its own
// cross-platform confirm rather than relying on RN's Alert alone.
export function confirmAsync(title: string, message: string, confirmLabel = 'Confirmer'): Promise<boolean> {
  if (Platform.OS === 'web') {
    return Promise.resolve(window.confirm(`${title}\n\n${message}`));
  }
  return new Promise((resolve) => {
    Alert.alert(title, message, [
      { text: 'Annuler', style: 'cancel', onPress: () => resolve(false) },
      { text: confirmLabel, style: 'destructive', onPress: () => resolve(true) },
    ]);
  });
}
