#!/usr/bin/env bash
# Run the Indra app on two iOS simulators simultaneously for P2P testing.
# Run from the repo root: ./bin/sim2.sh [--skip-build]
set -euo pipefail

ALICE_ID="9E08EFAB-6799-4AB3-8D30-C94A407C471E"   # iPhone 16 Pro (iOS 18.3)
BOB_ID="BBB05086-6890-4A18-AC1E-BD41D195DFD5"     # iPhone 16 Pro Max (iOS 18.3)

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APP_DIR="$REPO_ROOT/app"
IOS_DIR="$APP_DIR/ios"
WORKSPACE="$IOS_DIR/IndraApp.xcworkspace"
DERIVED_DATA="$HOME/Library/Developer/Xcode/DerivedData"
BUNDLE_ID="org.reactjs.native.example.IndraApp"

SKIP_BUILD=false
[[ "${1:-}" == "--skip-build" ]] && SKIP_BUILD=true

# ── 1. Start Metro in background ─────────────────────────────────────────────
echo "==> Starting Metro bundler..."
cd "$APP_DIR"
npx react-native start --port 8081 &
METRO_PID=$!
trap "kill $METRO_PID 2>/dev/null; exit" INT TERM EXIT

echo -n "    Waiting for Metro to be ready..."
for i in $(seq 1 30); do
  if curl -s -H "Host: localhost" http://localhost:8081/status | grep -q "packager-status:running" 2>/dev/null; then
    echo " ready."
    break
  fi
  sleep 1
  echo -n "."
done
echo ""

# ── 2. Build ──────────────────────────────────────────────────────────────────
if [[ "$SKIP_BUILD" == "false" ]]; then
  echo "==> Building IndraApp for simulator..."
  xcodebuild \
    -workspace "$WORKSPACE" \
    -scheme IndraApp \
    -configuration Debug \
    -destination "id=$ALICE_ID" \
    -derivedDataPath "$DERIVED_DATA" \
    build 2>&1 | grep -E "error:|warning:.*IndraModule|BUILD SUCCEEDED|BUILD FAILED" | grep -v "Werror"
  echo "==> Build complete."
fi

APP_PATH=$(find "$DERIVED_DATA" -name "IndraApp.app" -path "*/Debug-iphonesimulator/*" 2>/dev/null | head -1)
if [[ -z "$APP_PATH" ]]; then
  echo "ERROR: Could not find IndraApp.app. Run without --skip-build first."
  exit 1
fi
echo "==> App: $APP_PATH"

# ── 3. Boot simulators ────────────────────────────────────────────────────────
echo "==> Booting simulators..."
xcrun simctl boot "$ALICE_ID" 2>/dev/null || true
xcrun simctl boot "$BOB_ID"   2>/dev/null || true
open -a Simulator
sleep 3

# ── 4. Install & launch ───────────────────────────────────────────────────────
echo "==> Installing on Alice (iPhone 16 Pro)..."
xcrun simctl install "$ALICE_ID" "$APP_PATH"
xcrun simctl launch  "$ALICE_ID" "$BUNDLE_ID"

echo "==> Installing on Bob (iPhone 16 Pro Max)..."
xcrun simctl install "$BOB_ID" "$APP_PATH"
xcrun simctl launch  "$BOB_ID"  "$BUNDLE_ID"

echo ""
echo "==> Both simulators running. Metro is on http://localhost:8081"
echo "    Press Ctrl-C to stop Metro and exit."
wait $METRO_PID
