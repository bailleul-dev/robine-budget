import * as WebBrowser from 'expo-web-browser';
import { Platform } from 'react-native';
import { API_URL } from '@/api/client';

// Same custom scheme the app itself registers (see app.json's "scheme") —
// intercepted by the OS before it's ever loaded as a page, so this is only
// meaningful on native. It's the reverse-DNS bundle identifier on purpose:
// any app can claim a generic scheme and intercept the token in this
// redirect. Must match the backend's robineNativeCallbackURL allowlist.
const ROBINE_NATIVE_CALLBACK_URL = 'dev.bailleul.budget://auth-callback';

// On web, a custom scheme can't be navigated to in a browser tab/popup at
// all (that's what got this stuck in a popup before) — the backend needs a
// real same-origin page to redirect to instead. See
// mobile/src/app/auth-callback.tsx, which just calls
// expo-web-browser's maybeCompleteAuthSession() to hand the URL back to the
// tab that opened this popup and let it close itself.
function callbackUrl(): string {
  if (Platform.OS === 'web') {
    return `${window.location.origin}/auth-callback`;
  }
  return ROBINE_NATIVE_CALLBACK_URL;
}

/** Opens Robine ID's login page (in-app browser on native, a popup on web —
 * see expo-web-browser's openAuthSessionAsync) and resolves with the
 * session token once the backend hands it back (see backend's
 * RobineLogin/RobineCallback, which only accept this exact callback URL —
 * see isAllowedRobineCallback), or null if the user cancelled/dismissed.
 * Throws if the backend redirects back without a token (e.g. a login it
 * rejected outright). Robine ID has no native SDK/ID-token shortcut like
 * Google's (auth/google.ts) — this round trip through our own backend is
 * the only flow it has, web or native alike. */
export async function signInWithRobine(): Promise<string | null> {
  const callback = callbackUrl();
  const loginUrl = `${API_URL}/auth/oidc/robine/login?callback=${encodeURIComponent(callback)}`;
  const result = await WebBrowser.openAuthSessionAsync(loginUrl, callback);

  if (result.type !== 'success') return null;

  const token = new URL(result.url).searchParams.get('token');
  if (!token) {
    throw new Error('Robine ID login did not return a session token.');
  }
  return token;
}
