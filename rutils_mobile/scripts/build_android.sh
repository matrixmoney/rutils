#!/usr/bin/env bash
# ==============================================================================
# a2zutils Mobile - Android Build Script
# Builds Debug APK, Production Release APK, and Play Store AAB Bundle.
# ==============================================================================

set -e

BUILD_TYPE=${1:-"all"} # "apk", "aab", "debug", or "all"
PROD_API="https://utils.api.srilakshmiretail.in"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

GREEN='\033[0;32m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BOLD='\033[1m'
NC='\033[0m'

echo -e "${BLUE}======================================================${NC}"
echo -e "${GREEN}${BOLD} a2zutils Mobile — Android Build Engine ${NC}"
echo -e "${BLUE}======================================================${NC}"
echo -e "Target Application ID: ${CYAN}com.a2zutils${NC}"
echo -e "Production API Target: ${CYAN}$PROD_API${NC}"
echo -e "Build Target:          ${YELLOW}$BUILD_TYPE${NC}"
echo ""

cd "$APP_DIR"

build_debug_apk() {
    echo -e "${CYAN}==> Building Debug APK (points to local dev)...${NC}"
    flutter build apk --debug --dart-define=API_URL="http://10.0.2.2:8080"
    echo -e "${GREEN}✓ Debug APK built successfully:${NC}"
    ls -lh "$APP_DIR/build/app/outputs/flutter-apk/app-debug.apk"
    echo ""
}

build_release_apk() {
    echo -e "${CYAN}==> Building Production Release APK (for Direct Sideload / Distro)...${NC}"
    flutter build apk --release --dart-define=API_URL="$PROD_API"
    echo -e "${GREEN}✓ Release APK built successfully:${NC}"
    ls -lh "$APP_DIR/build/app/outputs/flutter-apk/app-release.apk"
    echo ""
}

build_release_aab() {
    echo -e "${CYAN}==> Building Google Play Store App Bundle (AAB)...${NC}"
    flutter build appbundle --release --dart-define=API_URL="$PROD_API"
    echo -e "${GREEN}✓ Play Store App Bundle (AAB) built successfully:${NC}"
    ls -lh "$APP_DIR/build/app/outputs/bundle/release/app-release.aab"
    echo ""
}

case "$BUILD_TYPE" in
    debug)
        build_debug_apk
        ;;
    apk)
        build_release_apk
        ;;
    aab)
        build_release_aab
        ;;
    all)
        build_release_apk
        build_release_aab
        ;;
    *)
        echo -e "${RED}Unknown build target: $BUILD_TYPE${NC}"
        echo "Usage: ./scripts/build_android.sh [all|apk|aab|debug]"
        exit 1
        ;;
esac

echo -e "${GREEN}${BOLD}All requested Android build artifacts completed!${NC}"
