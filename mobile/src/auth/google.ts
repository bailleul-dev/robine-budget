import * as Google from 'expo-auth-session/providers/google';
import * as WebBrowser from 'expo-web-browser';
import { Platform } from 'react-native';

// Required once at module scope by expo-auth-session's own docs — on
// native it closes the in-app browser and delivers the redirect back to
// the app; a no-op on web.
WebBrowser.maybeCompleteAuthSession();

// One Google Cloud OAuth client ID per platform — Google issues a
// different one for each (Web application, iOS, Android), and an ID token
// is only valid for the client it was requested with. Only webClientId is
// likely set today (this app is tested via `expo start --web`); iOS/Android
// need their own client created in Google Cloud Console before a native
// build can use this (see mobile/README or ask — a client already exists
// for the web app's own Google login, but that one's redirect flow is
// server-side and can't be reused here as-is).
const webClientId = process.env.EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID;
const iosClientId = process.env.EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID;
const androidClientId = process.env.EXPO_PUBLIC_GOOGLE_ANDROID_CLIENT_ID;

// Per-platform, not "is any client ID set anywhere" — expo-auth-session's
// useIdTokenAuthRequest throws synchronously during render if the ID for
// *this* platform is missing (see its invariantClientId), so the caller
// must only mount the component that calls useGoogleIdToken when this is
// true, never call the hook unconditionally and hide its result.
//
// Never on iOS: App Store guideline 4.8 requires offering Sign in with Apple
// alongside any third-party login like Google. iOS keeps email/password and
// Robine ID (our own identity provider, which the guideline exempts)
// instead. iosClientId is still passed below so the hook stays valid if
// that decision changes.
export const googleAuthConfigured =
  Platform.OS !== 'ios' && Boolean(Platform.select({ android: androidClientId, default: webClientId }));

/** Wraps expo-auth-session's Google provider so the login screen doesn't
 * need to know about client IDs or response shapes — just call
 * `promptAsync()` and await the id token. Only call this (i.e. only mount
 * whatever component calls it) when googleAuthConfigured is true. */
export function useGoogleIdToken() {
  const [request, response, promptAsync] = Google.useIdTokenAuthRequest({
    webClientId,
    iosClientId,
    androidClientId,
  });

  const idToken = response?.type === 'success' ? (response.params.id_token ?? response.authentication?.idToken) : undefined;

  return { request, response, promptAsync, idToken };
}
