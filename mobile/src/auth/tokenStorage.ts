/** Where the session token lives between app launches. Native has no
 * cookie jar the way a browser does, so the backend hands the token back
 * in the login/register response body (see backend's authResponse) and we
 * store it ourselves — SecureStore's Keychain/Keystore on iOS/Android,
 * localStorage on web (SecureStore doesn't exist there). */
import { Platform } from 'react-native';
import * as SecureStore from 'expo-secure-store';

const KEY = 'budget_session_token';

export async function getToken(): Promise<string | null> {
  if (Platform.OS === 'web') {
    return window.localStorage.getItem(KEY);
  }
  return SecureStore.getItemAsync(KEY);
}

export async function setToken(token: string): Promise<void> {
  if (Platform.OS === 'web') {
    window.localStorage.setItem(KEY, token);
    return;
  }
  await SecureStore.setItemAsync(KEY, token);
}

export async function clearToken(): Promise<void> {
  if (Platform.OS === 'web') {
    window.localStorage.removeItem(KEY);
    return;
  }
  await SecureStore.deleteItemAsync(KEY);
}
