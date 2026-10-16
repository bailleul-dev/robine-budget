import { Platform } from 'react-native';
import { clearToken, getToken } from '@/auth/tokenStorage';

/** Falls back to a platform-aware default so a fresh checkout works
 * without any setup for the two most common dev targets (Expo web, iOS
 * simulator — both reach the Mac's own localhost). Android's emulator
 * loopback is different (10.0.2.2), and a physical device needs the
 * host machine's LAN IP — set EXPO_PUBLIC_API_URL for either. */
function defaultApiUrl(): string {
  if (Platform.OS === 'android') return 'http://10.0.2.2:8000/api';
  return 'http://localhost:8000/api';
}

export const API_URL = process.env.EXPO_PUBLIC_API_URL ?? defaultApiUrl();

/** The origin part ("https://host") of an absolute URL, or null for a
 * relative one. A regex rather than `new URL()`: React Native's URL
 * polyfill doesn't implement `origin`. */
function originOf(url: string): string | null {
  return url.match(/^https?:\/\/[^/]+/)?.[0] ?? null;
}

function currentWebOrigin(): string | null {
  return Platform.OS === 'web' && typeof window !== 'undefined' ? window.location.origin : null;
}

/** API_URL as an absolute URL — the production web build uses a relative
 * "/api" (see the Dockerfile), which is fine for fetch() but not for a URL
 * shown to the user or pasted elsewhere (e.g. the MCP connector address). */
export const API_PUBLIC_URL = originOf(API_URL) ? API_URL : `${currentWebOrigin() ?? ''}${API_URL}`;

/** Where the web app lives, for links meant to be opened in a browser
 * (share invites → /invite/:token). In production the Go server serves the
 * web app on the same origin as the API, so: the page's own origin on web,
 * the API's origin on native. EXPO_PUBLIC_WEB_APP_URL overrides both, e.g.
 * for a native dev build whose web app runs on a separate dev server. */
export const WEB_APP_URL =
  process.env.EXPO_PUBLIC_WEB_APP_URL ?? currentWebOrigin() ?? originOf(API_URL) ?? 'http://localhost:8081';

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

/** Set once AuthContext knows the user is signed out, so a 401 from any
 * in-flight request can drop the (now-invalid) stored token without every
 * call site having to remember to do it. */
let onUnauthorized: (() => void) | null = null;
export function setUnauthorizedHandler(handler: (() => void) | null) {
  onUnauthorized = handler;
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const token = await getToken();
  const headers: Record<string, string> = { ...(options.headers as Record<string, string>) };
  if (options.body && !(options.body instanceof FormData)) {
    headers['Content-Type'] = 'application/json';
  }
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }

  const response = await fetch(`${API_URL}${path}`, { ...options, headers });

  if (response.status === 401) {
    await clearToken();
    onUnauthorized?.();
  }

  if (!response.ok) {
    let detail = response.statusText;
    try {
      const body = await response.json();
      detail = body.detail ?? detail;
    } catch {
      // response had no JSON body
    }
    throw new ApiError(response.status, detail);
  }

  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined }),
  put: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PUT', body: body ? JSON.stringify(body) : undefined }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
  // FormData body: request() above already skips the JSON Content-Type
  // header for it, so fetch sets its own multipart boundary.
  postForm: <T>(path: string, form: FormData) => request<T>(path, { method: 'POST', body: form }),
};
