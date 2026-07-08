import {
  Activity,
  GitBranch,
  Globe,
  LayoutDashboard,
  Radio,
  ScrollText,
  Settings,
  type LucideIcon,
} from 'lucide-react'

export interface NavItem {
  to: string
  label: string
  icon: LucideIcon
  end?: boolean
  description: string
}

export const NAV_ITEMS: NavItem[] = [
  {
    to: '/',
    label: 'Dashboard',
    icon: LayoutDashboard,
    end: true,
    description: 'Overview & quick actions',
  },
  {
    to: '/connections',
    label: 'Connections',
    icon: Activity,
    description: 'AmneziaWG & Xray tunnels',
  },
  {
    to: '/subscriptions',
    label: 'Subscriptions',
    icon: Globe,
    description: 'Xray subscription feeds',
  },
  {
    to: '/bypass',
    label: 'Bypass',
    icon: Radio,
    description: 'nfqws2 DPI bypass',
  },
  {
    to: '/failover',
    label: 'Failover',
    icon: GitBranch,
    description: 'Fallback chain & health',
  },
  {
    to: '/logs',
    label: 'Logs',
    icon: ScrollText,
    description: 'Live service logs',
  },
  {
    to: '/settings',
    label: 'Settings',
    icon: Settings,
    description: 'Web UI, auth & platform',
  },
]
