#!/usr/bin/env python3
"""
Validate the agent registry structure and content.
"""

import json
import sys
from pathlib import Path


def validate_registry(registry_path: Path) -> bool:
    """Validate registry structure and content."""
    errors = []
    warnings = []

    try:
        with open(registry_path, 'r', encoding='utf-8') as f:
            registry = json.load(f)
    except json.JSONDecodeError as e:
        print(f"❌ Invalid JSON: {e}")
        return False
    except FileNotFoundError:
        print(f"❌ Registry not found: {registry_path}")
        return False

    # Check version
    if 'version' not in registry:
        errors.append("Missing 'version' field")
    elif not isinstance(registry['version'], str):
        errors.append("'version' must be a string")

    # Check agents
    if 'agents' not in registry:
        errors.append("Missing 'agents' field")
        return False

    if not isinstance(registry['agents'], dict):
        errors.append("'agents' must be an object")
        return False

    # Validate each agent
    required_fields = ['name', 'path']
    optional_fields = ['model', 'color', 'description']
    valid_models = ['opus', 'sonnet', 'inherit']

    for agent_id, agent_data in registry['agents'].items():
        # Check required fields
        for field in required_fields:
            if field not in agent_data:
                errors.append(f"Agent '{agent_id}' missing required field: {field}")

        # Validate model if present
        if 'model' in agent_data:
            if agent_data['model'] not in valid_models:
                errors.append(f"Agent '{agent_id}' has invalid model: {agent_data['model']}")

        # Check path exists
        if 'path' in agent_data:
            agent_file = Path(__file__).parent.parent / agent_data['path']
            if not agent_file.exists():
                errors.append(f"Agent '{agent_id}' file not found: {agent_data['path']}")

        # Warn about missing optional but recommended fields
        if 'model' not in agent_data:
            warnings.append(f"Agent '{agent_id}' missing recommended field: model")
        if 'description' not in agent_data:
            warnings.append(f"Agent '{agent_id}' missing recommended field: description")

    # Print results
    print(f"\n{'='*60}")
    print(f"Agent Registry Validation")
    print(f"{'='*60}")
    print(f"Registry: {registry_path}")
    print(f"Version: {registry.get('version', 'N/A')}")
    print(f"Total Agents: {len(registry['agents'])}")
    print(f"{'='*60}\n")

    if errors:
        print(f"❌ ERRORS ({len(errors)}):")
        for error in errors:
            print(f"  - {error}")
        print()

    if warnings:
        print(f"⚠️  WARNINGS ({len(warnings)}):")
        for warning in warnings[:10]:  # Show first 10 warnings
            print(f"  - {warning}")
        if len(warnings) > 10:
            print(f"  ... and {len(warnings) - 10} more")
        print()

    if not errors and not warnings:
        print("✅ Registry is valid with no errors or warnings!")
    elif not errors:
        print("✅ Registry is valid (with warnings)")
    else:
        print("❌ Registry validation failed")

    return len(errors) == 0


def main():
    """Main entry point."""
    script_dir = Path(__file__).parent
    repo_root = script_dir.parent
    registry_path = repo_root / ".hanzo/agents/plugins" / "agent-registry.json"

    is_valid = validate_registry(registry_path)
    sys.exit(0 if is_valid else 1)


if __name__ == "__main__":
    main()
