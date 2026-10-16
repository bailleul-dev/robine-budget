#!/usr/bin/env bash
# App Store screenshots, end to end — `make ios-screenshots`:
#
#   1. a throwaway Budget server (its own database and port, never your dev
#      data) filled with a demo user by seed-demo.mjs;
#   2. a Release build of the app for the simulator, pointed at that server;
#   3. for each required display size, a clean simulator (9:41, full
#      battery, light mode) running flows/app-store.yaml through Maestro;
#   4. PNGs in fastlane/screenshots/fr-FR/, where `make ios-screenshots-upload`
#      (fastlane deliver) picks them up;
#   5. on success, a clean-up that keeps only those PNGs: the build (~5 GB),
#      Maestro's debug output, the demo database and simulator all go. A
#      failed run keeps everything for debugging.
#
# Needs: Docker (the repo's Postgres), Go, Xcode, Maestro
# (brew install mobile-dev-inc/tap/maestro).
#
# Env: KEEP_BUILD=1 skips deleting the simulator build, and SKIP_BUILD=1
# then reuses it (for iterating on flows/); SCREENSHOTS_PORT
# (8010) and SCREENSHOTS_DEVICES ("iPhone 17 Pro Max") override defaults.
set -euo pipefail

MOBILE="$(cd "$(dirname "$0")/.." && pwd)"
ROOT="$(cd "$MOBILE/.." && pwd)"
WORK="$MOBILE/build/screenshots"
OUT="$MOBILE/fastlane/screenshots/fr-FR"
PORT="${SCREENSHOTS_PORT:-8010}"
API_URL="http://127.0.0.1:$PORT/api"
DB=budget_screenshots
# Apple requires 6.9" iPhone screenshots (1320×2868); smaller iPhone sizes
# are scaled down from them. supportsTablet is false, so no iPad.
IFS=',' read -r -a DEVICES <<< "${SCREENSHOTS_DEVICES:-iPhone 17 Pro Max}"
APP="$WORK/DerivedData/Build/Products/Release-iphonesimulator/Lagune.app"

step() { printf '\n\033[1;36m→ %s\033[0m\n' "$*"; }

for tool in docker go xcodebuild xcrun maestro node; do
  command -v "$tool" >/dev/null || { echo "✖ $tool is required"; exit 1; }
done
mkdir -p "$WORK"

# --- 1. Demo server ---------------------------------------------------------
step "Fresh database $DB"
(cd "$ROOT" && docker compose up -d --wait postgres >/dev/null)
(cd "$ROOT" && docker compose exec -T postgres psql -q -U budget -d postgres \
  -c "DROP DATABASE IF EXISTS $DB WITH (FORCE)" -c "CREATE DATABASE $DB")

step "Demo server on :$PORT"
if lsof -iTCP:"$PORT" -sTCP:LISTEN -t >/dev/null 2>&1; then
  echo "✖ port $PORT is already in use (a previous run's server?) — free it or set SCREENSHOTS_PORT"
  exit 1
fi
(cd "$ROOT/backend" && go build -o "$WORK/server" ./cmd/server)
# Run outside backend/ so backend/.env (your dev settings) isn't loaded.
(cd "$WORK" && exec env -i PATH="$PATH" HOME="$HOME" \
  PORT="$PORT" \
  DATABASE_URL="postgres://budget:budget@localhost:5432/$DB?sslmode=disable" \
  JWT_SECRET=screenshots \
  ./server >"$WORK/server.log" 2>&1) &
SERVER_PID=$!
trap 'kill $SERVER_PID 2>/dev/null || true' EXIT
for _ in $(seq 1 60); do
  curl -sf "http://127.0.0.1:$PORT/health" >/dev/null && break
  kill -0 $SERVER_PID 2>/dev/null || { cat "$WORK/server.log"; exit 1; }
  sleep 1
done

step "Seeding the demo user"
API_URL="$API_URL" node "$MOBILE/screenshots/seed-demo.mjs"

# --- 2. Simulator build -----------------------------------------------------
if [[ "${SKIP_BUILD:-}" == "1" && -d "$APP" ]]; then
  step "Reusing $APP"
else
  step "Release build for the simulator (API $API_URL)"
  # prebuild --clean regenerates ios/ with this API URL; `make ios-beta`
  # regenerates it again with the production one.
  (cd "$MOBILE" && EXPO_PUBLIC_API_URL="$API_URL" NODE_ENV=production \
    npx expo prebuild --platform ios --clean)
  (cd "$MOBILE" && EXPO_PUBLIC_API_URL="$API_URL" NODE_ENV=production \
    xcodebuild -workspace ios/Lagune.xcworkspace -scheme Lagune \
      -configuration Release -sdk iphonesimulator \
      -destination 'generic/platform=iOS Simulator' \
      -derivedDataPath "$WORK/DerivedData" \
      CODE_SIGN_STYLE=Manual CODE_SIGN_IDENTITY=- DEVELOPMENT_TEAM= \
      build -quiet)
  # Ad hoc "Sign to Run Locally", not CODE_SIGNING_ALLOWED=NO: an unsigned
  # simulator app has no keychain entitlement, so expo-secure-store throws
  # and sign-in fails before it even reaches the server.
fi

# --- 3. Screenshots ---------------------------------------------------------
RUNTIME=$(xcrun simctl list runtimes -j | node -e '
  const rs = JSON.parse(require("fs").readFileSync(0)).runtimes.filter((r) => r.platform === "iOS" && r.isAvailable);
  console.log(rs.at(-1).identifier)')
rm -rf "$OUT" && mkdir -p "$OUT"
CREATED_DEVICES=()

for DEVICE in "${DEVICES[@]}"; do
  NAME="Lagune Screenshots ($DEVICE)"
  step "$DEVICE"
  UDID=$(xcrun simctl list devices -j | node -e '
    const name = process.argv[1];
    const all = Object.values(JSON.parse(require("fs").readFileSync(0)).devices).flat();
    console.log(all.find((d) => d.name === name && d.isAvailable)?.udid ?? "")' "$NAME")
  if [[ -z "$UDID" ]]; then
    TYPE="com.apple.CoreSimulator.SimDeviceType.${DEVICE// /-}"
    UDID=$(xcrun simctl create "$NAME" "$TYPE" "$RUNTIME")
  fi
  xcrun simctl boot "$UDID" 2>/dev/null || true
  xcrun simctl bootstatus "$UDID" -b >/dev/null
  xcrun simctl ui "$UDID" appearance light
  xcrun simctl status_bar "$UDID" override --time 9:41 --dataNetwork wifi --wifiMode active \
    --wifiBars 3 --cellularMode active --cellularBars 4 --batteryState charged --batteryLevel 100
  xcrun simctl uninstall "$UDID" dev.bailleul.budget 2>/dev/null || true
  xcrun simctl install "$UDID" "$APP"

  SHOTS="$WORK/shots/$DEVICE"
  rm -rf "$SHOTS" && mkdir -p "$SHOTS"
  maestro --device "$UDID" test --flatten-debug-output --debug-output "$SHOTS" \
    "$MOBILE/screenshots/flows/app-store.yaml"
  # Maestro 2 files takeScreenshot output under <debug-output>/<flow>/takeScreenshot/.
  find "$SHOTS" -path '*takeScreenshot*' -name '*.png' | while read -r png; do
    cp "$png" "$OUT/$DEVICE-$(basename "$png")"
  done
  xcrun simctl shutdown "$UDID"
  CREATED_DEVICES+=("$UDID")
done

# --- 5. Clean-up ------------------------------------------------------------
step "Cleaning up"
kill $SERVER_PID 2>/dev/null || true
wait $SERVER_PID 2>/dev/null || true
(cd "$ROOT" && docker compose exec -T postgres psql -q -U budget -d postgres \
  -c "DROP DATABASE IF EXISTS $DB WITH (FORCE)")
for UDID in "${CREATED_DEVICES[@]}"; do
  xcrun simctl delete "$UDID"
done
if [[ "${KEEP_BUILD:-}" == "1" ]]; then
  rm -rf "$WORK/shots" "$WORK/server" "$WORK/server.log"
else
  rm -rf "$WORK"
fi

step "Done"
ls -1 "$OUT"
