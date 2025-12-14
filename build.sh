#!/bin/bash
# ==================================================
#  UEM Deployment Tools - Clean Build Script
# ==================================================

# --- 0. Setup: Wails Path and Platform ---
WAILS_CMD="$HOME/go/bin/wails"
if ! command -v $WAILS_CMD &> /dev/null; then
    echo "ERROR: Wails CLI not found at $WAILS_CMD"
    exit 1
fi

PLATFORM_NAME="windows"
WAILS_PLATFORM="windows"
OUTPUT_NAME="uemtools.exe"

if [[ "$1" == "mac" ]]; then
    PLATFORM_NAME="mac"
    WAILS_PLATFORM="darwin"
    OUTPUT_NAME="UEM Deployment Tools.app"
fi

# Final, clean destination for all artifacts
FINAL_DIR="dist/$PLATFORM_NAME"

echo "=========================================="
echo "  Starting Clean Build"
echo "  Target: $PLATFORM_NAME"
echo "  Output: ./$FINAL_DIR/"
echo "=========================================="
echo

# --- 1. Clean Up ---
echo "[1/4] Cleaning up previous build artifacts..."
# Remove old messy directories from previous script versions
rm -rf build/mac build/darwin build/windows
# Create a fresh, empty destination directory
rm -rf "$FINAL_DIR"
mkdir -p "$FINAL_DIR"
echo "  -> Cleanup complete."
echo

# --- 2. Build Linux Agents ---
echo "[2/4] Building Linux Agents..."
GOOS=linux GOARCH=amd64 go build -o cncyagent_amd64 ./agent
if [ $? -ne 0 ]; then exit 1; fi
GOOS=linux GOARCH=arm64 go build -o cncyagent_arm64 ./agent
if [ $? -ne 0 ]; then exit 1; fi
echo "  -> Agents built."
echo

# --- 3. Build Wails Application ---
echo "[3/4] Building Wails Application ($PLATFORM_NAME)..."
# Wails builds into 'build/bin' by default. The -clean flag cleans this folder.
$WAILS_CMD build -clean -platform $WAILS_PLATFORM
if [ $? -ne 0 ]; then
    echo "ERROR: Wails build failed."
    rm -f cncyagent_amd64 cncyagent_arm64
    exit 1
fi
echo "  -> Wails app built."
echo

# --- 4. Assemble Final Artifacts ---
echo "[4/4] Assembling files in ./$FINAL_DIR/ ..."
# Move the main application from Wails' temp dir to our final dir
mv "build/bin/$OUTPUT_NAME" "$FINAL_DIR/"

# If Mac, also copy the inner binary to the root of dist/mac for convenience
if [[ "$PLATFORM_NAME" == "mac" ]]; then
    echo "  -> Copying inner binary from .app bundle..."
    cp "$FINAL_DIR/$OUTPUT_NAME/Contents/MacOS/uemtools" "$FINAL_DIR/uemtools"

    echo "  -> Bundling agents into .app..."
    cp cncyagent_amd64 "$FINAL_DIR/$OUTPUT_NAME/Contents/MacOS/"
    cp cncyagent_arm64 "$FINAL_DIR/$OUTPUT_NAME/Contents/MacOS/"
fi

# Move the agents to the final dir (keep external copy for reference/scripts)
# or remove this if we ONLY want them inside the app.
# The user said "don't need 3 packages", so maybe we don't need them outside?
# But for safety/Linux debug, keeping a copy outside might be useful, 
# or we can just copy them instead of move. 
# Let's COPY them to final dir as well, just in case user wants standalone.
cp cncyagent_amd64 "$FINAL_DIR/"
cp cncyagent_arm64 "$FINAL_DIR/"
echo "  -> Assembly complete."
echo

# --- 5. Final Report ---
echo "=========================================="
echo "  Build Successful!"
echo "=========================================="
echo "All artifacts are in: ./$FINAL_DIR/"
ls -l "$FINAL_DIR/"
echo
