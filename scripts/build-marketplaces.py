#!/usr/bin/env python3
"""Build marketplace JSON files from agent registry and common config.

This script generates all marketplace files from:
- agent-registry.json (canonical agent definitions)
- common-config.json (shared metadata)
- marketplace definitions (discipline groupings)
"""

import json
from pathlib import Path

# Load common config and registry
config_dir = Path(__file__).parent.parent / ".hanzo/agents/plugins"
common = json.loads((config_dir / "common-config.json").read_text())
registry = json.loads((config_dir / "agent-registry.json").read_text())

def create_plugin(name, description, agents, keywords, category):
    """Create a plugin definition using common config."""
    return {
        "name": name,
        "source": common["defaults"]["source"],
        "description": description,
        "version": common["version"],
        "author": common["defaults"]["author"],
        "homepage": common["defaults"]["homepage"],
        "repository": common["defaults"]["repository"],
        "license": common["defaults"]["license"],
        "keywords": keywords,
        "category": category,
        "strict": common["defaults"]["strict"],
        "agents": [registry["agents"][agent_id]["path"] for agent_id in agents if agent_id in registry["agents"]]
    }

def create_marketplace(name, description, plugins):
    """Create a marketplace file."""
    return {
        "name": name,
        "owner": common["defaults"]["owner"],
        "metadata": {
            "description": description,
            "version": common["version"]
        },
        "plugins": plugins
    }

# Define marketplaces by discipline
marketplaces = {
    "hanzo-leadership": {
        "description": "C-Suite and executive leadership - CEO, CTO, COO, VP",
        "plugins": [
            create_plugin(
                "hanzo-leadership-team",
                "Complete leadership team: CEO, CTO, COO, VP",
                ["vp", "cto", "observability-engineer"],  # COO uses observability-engineer
                ["leadership", "executive", "ceo", "cto", "coo", "vp"],
                "leadership"
            )
        ]
    },
    "hanzo-engineering": {
        "description": "Complete engineering team - All roles and specialists",
        "plugins": [
            create_plugin(
                "hanzo-engineering-team",
                "All engineering roles: Staff, Senior, Tech Lead, Full-Stack, Backend, Frontend, DevOps, Platform, Mobile, QA",
                [
                    "staff-engineer", "senior-engineer", "tech-lead", "full-stack-engineer",
                    "backend-engineer", "frontend-engineer", "devops", "devops-engineer",
                    "platform-engineer", "mobile-engineer", "qa-engineer", "dev",
                    "backend-architect", "frontend-developer", "cloud-architect",
                    "kubernetes-architect", "terraform-specialist", "deployment-engineer"
                ],
                ["engineering", "development", "backend", "frontend", "devops", "fullstack"],
                "engineering"
            )
        ]
    },
    "hanzo-data": {
        "description": "Data and ML team - Data Engineers, Scientists, ML Engineers, AI",
        "plugins": [
            create_plugin(
                "hanzo-data-team",
                "Complete data and ML team with ETL, analytics, and AI specialists",
                ["data-engineer", "data-scientist", "ml-engineer", "mlops-engineer", "ai-engineer", "prompt-engineer"],
                ["data", "ml", "ai", "analytics", "etl", "machine-learning"],
                "data"
            )
        ]
    },
    "hanzo-security": {
        "description": "Security team - Auditing, secure coding, compliance",
        "plugins": [
            create_plugin(
                "hanzo-security-team",
                "Complete security team with auditing and secure coding specialists",
                ["security-engineer", "security-auditor", "backend-security-coder", "frontend-security-coder", "mobile-security-coder"],
                ["security", "auditing", "owasp", "penetration-testing", "compliance"],
                "security"
            )
        ]
    },
    "hanzo-design": {
        "description": "Design team - UI/UX designers, visual validation",
        "plugins": [
            create_plugin(
                "hanzo-design-team",
                "UI/UX design team with visual validation specialists",
                ["ui-ux-designer", "ui-visual-validator"],
                ["design", "ui", "ux", "figma", "wireframes"],
                "design"
            )
        ]
    },
    "hanzo-operations": {
        "description": "Operations team - SRE, monitoring, incident response",
        "plugins": [
            create_plugin(
                "hanzo-operations-team",
                "Complete operations and SRE team",
                ["observability-engineer", "incident-responder", "network-engineer", "performance-engineer"],
                ["operations", "sre", "monitoring", "observability", "incident-response"],
                "operations"
            )
        ]
    },
    "hanzo-marketing": {
        "description": "Marketing team - Content, SEO, social media",
        "plugins": [
            create_plugin(
                "hanzo-marketing-team",
                "Complete marketing and SEO team",
                [
                    "content-marketer", "seo-content-writer", "seo-content-auditor",
                    "seo-keyword-strategist", "seo-meta-optimizer", "seo-structure-architect",
                    "seo-snippet-hunter", "seo-content-refresher", "seo-cannibalization-detector",
                    "seo-authority-builder", "seo-content-planner", "sales-automator"
                ],
                ["marketing", "seo", "content", "social-media"],
                "marketing"
            )
        ]
    },
    "hanzo-finance": {
        "description": "Finance team - Quant, risk, trading, payments",
        "plugins": [
            create_plugin(
                "hanzo-finance-team",
                "Finance and trading team",
                ["quant-analyst", "risk-manager", "payment-integration", "business-analyst"],
                ["finance", "quant", "trading", "risk", "payments"],
                "finance"
            )
        ]
    },
    "hanzo-compliance": {
        "description": "Compliance team - Legal, HR, regulatory",
        "plugins": [
            create_plugin(
                "hanzo-compliance-team",
                "Legal and HR compliance specialists",
                ["legal-advisor", "hr-pro"],
                ["compliance", "legal", "hr", "gdpr", "soc2", "hipaa"],
                "compliance"
            )
        ]
    },
    "hanzo-support": {
        "description": "Support team - Customer support and success",
        "plugins": [
            create_plugin(
                "hanzo-support-team",
                "Customer support specialists",
                ["customer-support"],
                ["support", "customer-service", "troubleshooting"],
                "support"
            )
        ]
    }
}

# Generate marketplace files
for marketplace_name, config in marketplaces.items():
    marketplace_data = create_marketplace(
        marketplace_name,
        config["description"],
        config["plugins"]
    )

    output_file = config_dir / f"{marketplace_name}.json"
    output_file.write_text(json.dumps(marketplace_data, indent=2))
    print(f"✅ Generated {marketplace_name}.json")

print(f"\n🎉 Generated {len(marketplaces)} marketplace files")
