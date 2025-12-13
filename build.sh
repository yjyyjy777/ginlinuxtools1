#!/bin/bash
# ==================================================
#  UEM Deployment Tools - Unified Build Script
# ==================================================

# --- Ensure Go binaries are in PATH ---
# Dynamically find GOPATH using the go command
if command -v go &> /dev/null; then
    GOPATH=$(go env GOPATH)
    if [ -z "$GOPATH" ]; then
        GOPATH=$HOME/go
    fi
    export PATH=$PATH:$GOPATH/bin
else
    echo "WARNING: 'go' command not found. Make sure Go is installed and in your PATH."
fi

# Check if wails is found
if ! command -v wails &> /dev/null; then
    echo "ERROR: 'wails' command not found."
    echo "Current PATH: $PATH"
    echo "Please ensure Wails is installed (go install github.com/wailsapp/wails/v2/cmd/wails@latest) and in your PATH."
    exit 1
fi

# --- Default settings ---
PLATFORM=""
BUILD_AGENT=false
OUTPUT_NAME=""

# --- Parse command-line arguments ---
while [[ "$#" -gt 0 ]]; do
    case $1 in
        -os)
            PLATFORM="$2"
            shift
            ;;
        -build-agent)
            BUILD_AGENT=true
            ;;
        *)
            echo "Unknown parameter passed: $1"
            exit 1
            ;;
    esac
    shift
done

echo "Starting build process..."
echo "Target Platform: ${PLATFORM:-none}"
echo "Build Linux Agents: $BUILD_AGENT"
echo

# --- Step 1: Build Linux Agents (if requested) ---
if [ "$BUILD_AGENT" = true ]; then
    echo "Building Linux agent for amd64..."
    GOOS=linux GOARCH=amd64 go build -o cncyagent_amd64 ./agent
    if [ $? -ne 0 ]; then
        echo "ERROR: Failed to build Linux amd64 agent."
        exit 1
    fi
    echo " -> Successfully created 'cncyagent_amd64'"
    echo

    echo "Building Linux agent for arm64..."
    GOOS=linux GOARCH=arm64 go build -o cncyagent_arm64 ./agent
    if [ $? -ne 0 ]; then
        echo "ERROR: Failed to build Linux arm64 agent."
        rm -f cncyagent_amd64 # Clean up the previous binary
        exit 1
    fi
    echo " -> Successfully created 'cncyagent_arm64'"
    echo

    echo "Moving agent binaries to ./build/bin/ ..."
    # Ensure the build/bin directory exists
    mkdir -p ./build/bin/
    mv cncyagent_amd64 ./build/bin/
    mv cncyagent_arm64 ./build/bin/
    echo " -> Agent binaries moved."
    echo
fi

# --- Step 2: Build Wails Application for the target platform ---
if [ -n "$PLATFORM" ]; then
    if [ "$PLATFORM" == "windows" ]; then
        echo "Building Windows Wails application (uemtools.exe)..."
        wails build -clean -platform windows
        OUTPUT_NAME="uemtools.exe"
    elif [ "$PLATFORM" == "mac" ]; then
        echo "Building macOS Wails application (uemtools.app)..."
        wails build -clean -platform darwin
        OUTPUT_NAME="uemtools.app"
    else
        echo "ERROR: Unsupported OS '$PLATFORM'. Please use 'windows' or 'mac'."
        exit 1
    fi

    if [ $? -ne 0 ]; then
        echo "ERROR: Failed to build Wails application for $PLATFORM."
        exit 1
    fi
    echo " -> Successfully created 'build/bin/$OUTPUT_NAME'"
    echo

    # --- Step 3: Post-build steps for macOS ---
    if [ "$PLATFORM" == "mac" ]; then
        echo "Copying Linux agents into the macOS app bundle..."
        APP_BUNDLE_PATH="./build/bin/$OUTPUT_NAME"
        MACOS_DIR="$APP_BUNDLE_PATH/Contents/MacOS"

        if [ -d "$MACOS_DIR" ]; then
            cp ./build/bin/cncyagent_amd64 "$MACOS_DIR/"
            cp ./build/bin/cncyagent_arm64 "$MACOS_DIR/"
            echo " -> Agents successfully copied to $MACOS_DIR"
        else
            echo "ERROR: Could not find the MacOS directory in the app bundle: $MACOS_DIR"
            exit 1
        fi
        echo
    fi
fi

# --- Final Success Message ---
echo "=================================="
echo "  Build Process Finished!"
echo "=================================="

if [ "$BUILD_AGENT" = true ]; then
    echo "Linux Agent artifacts are in: ./build/bin/"
    echo " - cncyagent_amd64 (Linux Agent x86_64)"
    echo " - cncyagent_arm64 (Linux Agent ARM64)"
fi

if [ -n "$PLATFORM" ]; then
    echo "Wails application artifact is in: ./build/bin/"
    echo " - $OUTPUT_NAME ($PLATFORM Client)"
fi
echo
