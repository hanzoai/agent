#!/usr/bin/env bash
# Hanzo Agents CLI Uninstaller
# Usage: curl -fsSL https://hanzo-agents.ai/uninstall.sh | bash
# Or: bash scripts/uninstall.sh

set -e

# Configuration
INSTALL_DIR="${HANZO_AGENTS_INSTALL_DIR:-$HOME/.hanzo-agents}"
VERBOSE="${VERBOSE:-0}"

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# Print functions
print_info() {
  echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
  echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_error() {
  echo -e "${RED}[ERROR]${NC} $1" >&2
}

print_warning() {
  echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_verbose() {
  if [[ "$VERBOSE" == "1" ]]; then
    echo -e "${CYAN}[VERBOSE]${NC} $1"
  fi
}

print_banner() {
  echo ""
  echo -e "${CYAN}╔══════════════════════════════════════════════════════════════╗${NC}"
  echo -e "${CYAN}║${NC}           ${BOLD}Hanzo Agents CLI Uninstaller${NC}                      ${CYAN}║${NC}"
  echo -e "${CYAN}╚══════════════════════════════════════════════════════════════╝${NC}"
  echo ""
}

# Remove Hanzo Agents directory
remove_hanzo_agents_dir() {
  if [[ -d "$INSTALL_DIR" ]]; then
    print_info "Removing $INSTALL_DIR..."
    rm -rf "$INSTALL_DIR"
    print_success "Hanzo Agents directory removed"
  else
    print_warning "Hanzo Agents directory not found: $INSTALL_DIR"
  fi
}

# Remove PATH configuration
remove_path_config() {
  print_info "Checking shell configuration files..."

  local shell_name
  shell_name=$(basename "$SHELL")

  local shell_configs=()

  case "$shell_name" in
    bash)
      shell_configs+=("$HOME/.bashrc" "$HOME/.bash_profile")
      ;;
    zsh)
      shell_configs+=("$HOME/.zshrc")
      ;;
    fish)
      shell_configs+=("$HOME/.config/fish/config.fish")
      ;;
  esac

  local removed=false

  for config in "${shell_configs[@]}"; do
    if [[ -f "$config" ]]; then
      print_verbose "Checking $config..."

      # Check if file contains Hanzo Agents PATH
      if grep -q "\.hanzo-agents" "$config" 2>/dev/null; then
        print_info "Found Hanzo Agents PATH in $config"

        # Create backup
        cp "$config" "$config.bak.hanzo-agents"
        print_verbose "Created backup: $config.bak.hanzo-agents"

        # Remove Hanzo Agents PATH entries
        # This removes lines containing .hanzo-agents and the comment line before it
        sed -i.tmp '/# Hanzo Agents CLI/d; /\.hanzo-agents/d' "$config"
        rm -f "$config.tmp"

        print_success "Removed PATH configuration from $config"
        removed=true
      fi
    fi
  done

  if [[ "$removed" == "false" ]]; then
    print_info "No PATH configuration found in shell config files"
  fi
}

# Main uninstall flow
main() {
  print_banner

  # Check if Hanzo Agents is installed
  if [[ ! -d "$INSTALL_DIR" ]]; then
    print_warning "Hanzo Agents does not appear to be installed at $INSTALL_DIR"
    read -p "Continue anyway? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
      print_info "Uninstall cancelled"
      exit 0
    fi
  fi

  # Confirm uninstall
  echo -e "${BOLD}This will remove:${NC}"
  echo "  - Hanzo Agents directory: $INSTALL_DIR"
  echo "  - PATH configuration from shell config files"
  echo ""
  read -p "Are you sure you want to uninstall Hanzo Agents? (y/N) " -n 1 -r
  echo
  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    print_info "Uninstall cancelled"
    exit 0
  fi

  echo ""

  # Remove Hanzo Agents directory
  remove_hanzo_agents_dir

  # Remove PATH configuration
  remove_path_config

  # Print success message
  echo ""
  echo -e "${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
  echo -e "${GREEN}║${NC}  ${BOLD}Hanzo Agents CLI uninstalled successfully!${NC}                  ${GREEN}║${NC}"
  echo -e "${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}"
  echo ""
  echo -e "${BOLD}Next steps:${NC}"
  echo ""
  echo "  1. Restart your terminal or reload your shell configuration"
  echo ""
  echo "  2. If you have backup files, you can restore them:"
  echo -e "     ${CYAN}ls ~/*.bak.hanzo-agents${NC}"
  echo ""
  echo "  3. To reinstall Hanzo Agents:"
  echo -e "     ${CYAN}curl -fsSL https://hanzo-agents.ai/install.sh | bash${NC}"
  echo ""
  echo "Thank you for using Hanzo Agents!"
  echo ""
}

# Run main function
main "$@"
