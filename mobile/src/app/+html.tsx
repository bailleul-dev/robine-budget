import { ScrollViewStyleReset } from 'expo-router/html';
import { type PropsWithChildren } from 'react';

// Root HTML shell for Expo Router's web build — only ever runs in Node, no
// browser APIs (see Expo Router's static-rendering docs). Without this file
// the dev/export server falls back to an empty <title> and no favicon link
// at all (confirmed via `curl localhost:8081/`), which is why the browser
// tab showed a blank title and generic icon. Title and favicon match the
// web app's own index.html so the two feel like the same product.
//
// The apple-mobile-web-app-* meta tags + manifest below are what make
// iOS Safari's "Add to Home Screen" behave like an installed app (its own
// icon, no browser chrome/URL bar) instead of a plain bookmark — see
// public/manifest.json and the generated public/apple-touch-icon.png,
// public/icon-{192,512}.png (all derived from assets/images/icon.png).
//
// IMPORTANT: this file is only rendered when app.json's web.output is
// "static" (confirmed by diffing `expo export --platform web` and the dev
// server's own HTML with output "single" vs "static" — "single" always
// serves Expo's generic template and silently ignores +html.tsx entirely,
// in both dev and export). Don't flip web.output back to "single" without
// re-adding this some other way.
export default function Root({ children }: PropsWithChildren) {
  return (
    <html lang="fr">
      <head>
        <meta charSet="utf-8" />
        <meta httpEquiv="X-UA-Compatible" content="IE=edge" />
        {/* viewport-fit=cover is what makes env(safe-area-inset-*) resolve
         * to a real, non-zero value at all in standalone/PWA mode on iOS —
         * without it the page never draws under the status bar/notch, so
         * SafeAreaProvider (see root _layout.tsx) has nothing to pad
         * against and that area just shows through as plain background. */}
        <meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no, viewport-fit=cover" />
        <title>Lagune</title>
        <link rel="icon" type="image/svg+xml" href="/favicon.svg" />
        <link rel="apple-touch-icon" href="/apple-touch-icon.png" />
        <link rel="manifest" href="/manifest.json" />
        <meta name="theme-color" content="#f3f6f8" />
        <meta name="mobile-web-app-capable" content="yes" />
        <meta name="apple-mobile-web-app-capable" content="yes" />
        <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent" />
        <meta name="apple-mobile-web-app-title" content="Lagune" />
        {/* Matches Colors.light/dark.background (constants/theme.ts) — the
         * status-bar/notch strip in standalone mode is *this* background
         * until React mounts and SafeAreaProvider-aware content paints
         * behind it, not the browser's own default white. */}
        <style>{`
          html, body { background: #f3f6f8; }
          @media (prefers-color-scheme: dark) {
            html, body { background: #0d1820; }
          }
        `}</style>
        <ScrollViewStyleReset />
        {/* ScrollViewStyleReset (above) sets html/body/#root to height:100%.
         * Installed on the iOS home screen (standalone, viewport-fit=cover,
         * black-translucent status bar), iOS reports a viewport shorter
         * than the screen by about the status bar's height — 100%, 100vh
         * and 100dvh alike — so the app stopped short of the bottom edge,
         * with the tab bar floating above a strip of plain background. In
         * that mode the page always covers the whole screen, so the script
         * sizes it from `screen` instead (portrait: the longer side), kept
         * up to date on rotation. iPhone only: an iPad app can share the screen
         * in Split View. Everywhere else --app-height stays unset
         * and the dvh/100% fallbacks apply. Placed after
         * ScrollViewStyleReset's <style> so it wins the cascade. */}
        <style>{`
          html, body, #root { height: var(--app-height, 100%); }
          @supports (height: 100dvh) {
            html, body, #root { height: var(--app-height, 100dvh); }
          }
        `}</style>
        <script
          dangerouslySetInnerHTML={{
            __html: `(function () {
  var standalone = window.navigator.standalone === true || window.matchMedia('(display-mode: standalone)').matches;
  if (!standalone || !/iPhone|iPod/.test(navigator.userAgent)) return;
  function fit() {
    var portrait = window.matchMedia('(orientation: portrait)').matches;
    var long = Math.max(screen.width, screen.height), short = Math.min(screen.width, screen.height);
    document.documentElement.style.setProperty('--app-height', (portrait ? long : short) + 'px');
  }
  fit();
  window.addEventListener('resize', fit);
  window.addEventListener('orientationchange', fit);
})();`,
          }}
        />
      </head>
      <body>{children}</body>
    </html>
  );
}
