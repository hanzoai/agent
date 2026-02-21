import type { NavigationSection } from '@/components/Navigation/types';

export const navigationSections: NavigationSection[] = [
  {
    id: 'playground',
    title: 'Playground',
    items: [
      {
        id: 'agents',
        label: 'Playground',
        href: '/playground',
        icon: 'bot',
        description: 'Organize, launch, watch, and control bots in one place'
      },
      {
        id: 'market',
        label: 'Marketplace',
        href: '/market',
        icon: 'package',
        description: 'Discover, deploy, and monetize bots'
      }
    ]
  },
  {
    id: 'admin-overview',
    title: 'Admin',
    items: [
      {
        id: 'dashboard',
        label: 'Dashboard',
        href: '/dashboard',
        icon: 'dashboard',
        description: 'Real-time system overview and metrics'
      },
      {
        id: 'node-overview',
        label: 'Nodes',
        href: '/nodes',
        icon: 'data-center',
        description: 'Node infrastructure and status'
      },
      {
        id: 'all-reasoners',
        label: 'Bots',
        href: '/reasoners/all',
        icon: 'function',
        description: 'Browse and manage all bots'
      }
    ]
  },
  {
    id: 'executions',
    title: 'Executions',
    items: [
      {
        id: 'individual-executions',
        label: 'Executions',
        href: '/executions',
        icon: 'run',
        description: 'Individual bot executions'
      },
      {
        id: 'workflow-executions',
        label: 'Workflows',
        href: '/workflows',
        icon: 'flow-data',
        description: 'Multi-step workflow processes'
      }
    ]
  },
  {
    id: 'identity-trust',
    title: 'Identity',
    items: [
      {
        id: 'did-explorer',
        label: 'DID Explorer',
        href: '/identity/dids',
        icon: 'identification',
        description: 'Decentralized identifiers for bots'
      },
      {
        id: 'credentials',
        label: 'Credentials',
        href: '/identity/credentials',
        icon: 'shield-check',
        description: 'Verify execution credentials'
      }
    ]
  },
  {
    id: 'settings',
    title: 'Settings',
    items: [
      {
        id: 'observability-webhook',
        label: 'Webhooks',
        href: '/settings/observability-webhook',
        icon: 'settings',
        description: 'Configure event forwarding'
      }
    ]
  }
];
