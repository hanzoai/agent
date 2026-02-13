#!/usr/bin/env python3
"""
Generate agent registry from agent markdown files.
Scans all .md files in agents/ and extracts frontmatter metadata.
"""

import json
import re
from pathlib import Path
from typing import Dict, Any


def parse_frontmatter(content: str) -> Dict[str, str]:
    """Extract frontmatter from markdown content."""
    # Match YAML frontmatter between --- delimiters
    match = re.match(r'^---\s*\n(.*?)\n---', content, re.DOTALL)
    if not match:
        return {}

    frontmatter = {}
    yaml_content = match.group(1)

    # Parse simple YAML fields (name, model, color, description)
    for line in yaml_content.split('\n'):
        # Handle multi-line description
        if line.startswith('description:'):
            # Extract description text (can be multi-line)
            desc_match = re.search(r'description:\s*(.+?)(?=\nmodel:|$)', yaml_content, re.DOTALL)
            if desc_match:
                # Clean up the description
                desc = desc_match.group(1).strip()
                # Remove quotes if present
                desc = desc.strip('"').strip("'")
                frontmatter['description'] = desc
        elif ':' in line and not line.strip().startswith('-'):
            key, value = line.split(':', 1)
            key = key.strip()
            value = value.strip().strip('"').strip("'")
            if key in ['name', 'model', 'color'] and value:
                frontmatter[key] = value

    return frontmatter


def generate_registry(agents_dir: Path) -> Dict[str, Any]:
    """Scan all agent files and generate registry."""
    registry = {
        "version": "1.0.1",
        "agents": {}
    }

    # Find all .md files in agents directory
    agent_files = sorted(agents_dir.glob("*.md"))

    for agent_file in agent_files:
        # Skip README
        if agent_file.name == "README.md":
            continue

        # Read file content
        try:
            content = agent_file.read_text(encoding='utf-8')
        except Exception as e:
            print(f"Warning: Could not read {agent_file}: {e}")
            continue

        # Parse frontmatter
        frontmatter = parse_frontmatter(content)

        if not frontmatter.get('name'):
            print(f"Warning: No name found in {agent_file}")
            continue

        agent_name = frontmatter['name']

        # Build agent entry
        agent_entry = {
            "name": agent_name.title() if agent_name.islower() else agent_name,
            "path": f"./agents/{agent_file.name}",
        }

        # Add optional fields
        if 'model' in frontmatter:
            agent_entry['model'] = frontmatter['model']

        if 'color' in frontmatter:
            agent_entry['color'] = frontmatter['color']

        if 'description' in frontmatter:
            # Truncate description to first sentence or 200 chars
            desc = frontmatter['description']
            # Get first sentence
            first_sentence = re.split(r'[.!?]\s', desc)[0]
            if len(first_sentence) < 200:
                agent_entry['description'] = first_sentence
            else:
                agent_entry['description'] = desc[:200] + "..."

        registry['agents'][agent_name] = agent_entry

    return registry


def main():
    """Main entry point."""
    # Get agents directory
    script_dir = Path(__file__).parent
    repo_root = script_dir.parent
    agents_dir = repo_root / ".hanzo" / "agents" / "definitions"

    if not agents_dir.exists():
        print(f"Error: Agents directory not found: {agents_dir}")
        return 1

    print(f"Scanning agents in: {agents_dir}")

    # Generate registry
    registry = generate_registry(agents_dir)

    print(f"Found {len(registry['agents'])} agents")

    # Create .hanzo/agents/plugins directory
    plugin_dir = repo_root / ".hanzo/agents/plugins"
    plugin_dir.mkdir(exist_ok=True)

    # Write registry
    registry_path = plugin_dir / "agent-registry.json"
    with open(registry_path, 'w', encoding='utf-8') as f:
        json.dump(registry, f, indent=2, ensure_ascii=False)

    print(f"Registry written to: {registry_path}")
    print(f"Total agents: {len(registry['agents'])}")

    return 0


if __name__ == "__main__":
    exit(main())
