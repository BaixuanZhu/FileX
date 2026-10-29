#!/bin/bash
set -e

# FileX Installation Script for Linux/macOS
# Usage: curl -fsSL https://raw.githubusercontent.com/BaixuanZhu/FileX/main/install.sh | bash

REPO_OWNER="BaixuanZhu"
REPO_NAME="FileX"
INSTALL_DIR="${HOME}/.filex"
WRAPPER_DIR="${HOME}/.local/bin"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

info() {
    echo -e "${CYAN}[INFO]${NC} $1"
}

success() {
    echo -e "${GREEN}[OK]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

# Detect platform
detect_platform() {
    local os=$(uname -s | tr '[:upper:]' '[:lower:]')
    local arch=$(uname -m)
    
    case "$os" in
        linux)
            PLATFORM="linux"
            ;;
        darwin)
            PLATFORM="darwin"
            ;;
        *)
            error "Unsupported operating system: $os"
            ;;
    esac
    
    case "$arch" in
        x86_64|amd64)
            ARCH="x64"
            ;;
        *)
            error "Unsupported architecture: $arch"
            ;;
    esac
    
    info "Detected platform: ${PLATFORM}-${ARCH}"
}

# Get latest release version
get_latest_version() {
    local url="https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest"
    local version=$(curl -s "$url" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    
    if [ -z "$version" ]; then
        error "Failed to fetch latest version"
    fi
    
    echo "$version"
}

# Download file
download_file() {
    local url="$1"
    local output="$2"
    
    info "Downloading from $url..."
    if ! curl -fsSL "$url" -o "$output"; then
        error "Download failed"
    fi
}

# Verify checksum
verify_checksum() {
    local file="$1"
    local expected="$2"
    
    info "Verifying checksum..."
    local actual=$(shasum -a 256 "$file" | cut -d' ' -f1)
    
    if [ "$actual" != "$expected" ]; then
        error "Checksum mismatch!\nExpected: $expected\nActual: $actual"
    fi
    
    success "Checksum verified"
}

# Add to PATH
add_to_path() {
    local dir="$1"
    local shell_rc=""
    
    # Detect shell
    if [ -n "$ZSH_VERSION" ]; then
        shell_rc="${HOME}/.zshrc"
    elif [ -n "$BASH_VERSION" ]; then
        shell_rc="${HOME}/.bashrc"
    else
        shell_rc="${HOME}/.profile"
    fi
    
    # Check if already in PATH
    if echo "$PATH" | grep -q "$dir"; then
        info "$dir is already in PATH"
        return
    fi
    
    # Add to PATH in shell config
    local path_line="export PATH=\"$dir:\$PATH\""
    if ! grep -q "$path_line" "$shell_rc" 2>/dev/null; then
        echo "" >> "$shell_rc"
        echo "# FileX" >> "$shell_rc"
        echo "$path_line" >> "$shell_rc"
        success "Added $dir to PATH in $shell_rc"
        info "Restart your shell or run: source $shell_rc"
    fi
}

# Main installation
install_filex() {
    info "Installing FileX..."
    
    # Detect platform
    detect_platform
    
    # Get latest version
    local version=$(get_latest_version)
    local version_number=${version#v}
    info "Latest version: $version"
    
    # Determine archive name
    local archive_name="filex-${version}-${PLATFORM}-${ARCH}.tar.gz"
    local download_url="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${version}/${archive_name}"
    
    # Create temporary directory
    local temp_dir=$(mktemp -d)
    trap "rm -rf $temp_dir" EXIT
    
    # Download archive
    local archive_path="${temp_dir}/${archive_name}"
    download_file "$download_url" "$archive_path"
    
    # Download and verify checksum
    local checksum_url="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${version}/checksums.txt"
    local checksum_path="${temp_dir}/checksums.txt"
    download_file "$checksum_url" "$checksum_path"
    
    local expected_checksum=$(grep "$archive_name" "$checksum_path" | cut -d' ' -f1)
    if [ -n "$expected_checksum" ]; then
        verify_checksum "$archive_path" "$expected_checksum"
    fi
    
    # Extract archive
    info "Extracting archive..."
    if ! tar -xzf "$archive_path" -C "$temp_dir"; then
        error "Failed to extract archive"
    fi
    
    # Find extracted directory
    local extracted_dir=$(find "$temp_dir" -maxdepth 1 -type d -name "filex-*" | head -n 1)
    if [ -z "$extracted_dir" ]; then
        error "Extracted directory not found"
    fi
    
    # Create installation directory structure
    local version_dir="${INSTALL_DIR}/versions/v${version_number}"
    if [ -d "$version_dir" ]; then
        rm -rf "$version_dir"
    fi
    mkdir -p "$version_dir"
    
    # Move binary and docs
    info "Installing to $version_dir..."
    mv "${extracted_dir}/filex" "$version_dir/"
    chmod +x "${version_dir}/filex"
    
    if [ -d "${extracted_dir}/docs" ]; then
        cp -r "${extracted_dir}/docs" "$version_dir/"
    fi
    if [ -f "${extracted_dir}/README.md" ]; then
        cp "${extracted_dir}/README.md" "$version_dir/"
    fi
    if [ -f "${extracted_dir}/LICENSE" ]; then
        cp "${extracted_dir}/LICENSE" "$version_dir/"
    fi
    
    # Create current symlink
    local current_link="${INSTALL_DIR}/current"
    if [ -L "$current_link" ] || [ -d "$current_link" ]; then
        rm -f "$current_link"
    fi
    
    info "Creating current link..."
    ln -s "$version_dir" "$current_link"
    success "Current version linked"
    
    # Create wrapper script
    mkdir -p "$WRAPPER_DIR"
    local wrapper_path="${WRAPPER_DIR}/filex"
    cat > "$wrapper_path" << 'EOF'
#!/bin/bash
set -e

FILEX_HOME="${HOME}/.filex"
FILEX_CURRENT="${FILEX_HOME}/current"

if [ ! -f "${FILEX_CURRENT}/filex" ]; then
    echo "Error: FileX current version not found"
    echo "Please run: filex upgrade"
    exit 1
fi

exec "${FILEX_CURRENT}/filex" "$@"
EOF
    chmod +x "$wrapper_path"
    success "Wrapper script created"
    
    # Add wrapper location to PATH
    add_to_path "$WRAPPER_DIR"
    
    # Verify installation
    info "Verifying installation..."
    local installed_binary="${current_link}/filex"
    if ! "$installed_binary" version > /dev/null 2>&1; then
        error "Installation verification failed"
    fi
    
    local installed_version=$("$installed_binary" version | grep -o '"version":"[^"]*"' | cut -d'"' -f4)
    success "FileX ${installed_version} installed successfully!"
    
    echo ""
    success "Installation complete!"
    echo ""
    echo "Installation structure:"
    echo "  ${INSTALL_DIR}/"
    echo "  ├── versions/"
    echo "  │   └── v${version_number}/"
    echo "  │       ├── filex"
    echo "  │       ├── README.md"
    echo "  │       ├── LICENSE"
    echo "  │       └── docs/"
    echo "  └── current -> versions/v${version_number}/"
    echo ""
    echo "Wrapper: ${wrapper_path}"
    echo ""
    echo "Usage:"
    echo "  filex help              Show help"
    echo "  filex version           Show version"
    echo "  filex upgrade           Upgrade to latest version"
    echo "  filex upgrade --list    List installed versions"
    echo "  filex upgrade --use v${version_number}    Switch to version"
    echo "  filex upgrade --cleanup    Remove old versions"
    echo ""
    echo "Documentation: https://github.com/${REPO_OWNER}/${REPO_NAME}"
    echo ""
}

# Run installation
install_filex
