import {
  Activity,
  GitBranch,
  Globe,
  LayoutDashboard,
  Radio,
  ScrollText,
  Settings,
  Waypoints,
  type LucideIcon,
} from 'lucide-react'

export interface NavItem {
  to: string
  /** i18n key for the sidebar label (resolved via useT). */
  labelKey: string
  /** i18n key for the description shown in tooltips / command palette. */
  descKey: string
  icon: LucideIcon
  end?: boolean
}

export const NAV_ITEMS: NavItem[] = [
  {
    to: '/',
    labelKey: 'nav.dashboard',
    descKey: 'nav.dashboardDesc',
    icon: LayoutDashboard,
    end: true,
  },
  {
    to: '/connections',
    labelKey: 'nav.connections',
    descKey: 'nav.connectionsDesc',
    icon: Activity,
  },
  {
    to: '/subscriptions',
    labelKey: 'nav.subscriptions',
    descKey: 'nav.subscriptionsDesc',
    icon: Globe,
  },
  {
    to: '/routes',
    labelKey: 'nav.routes',
    descKey: 'nav.routesDesc',
    icon: Waypoints,
  },
  {
    to: '/bypass',
    labelKey: 'nav.bypass',
    descKey: 'nav.bypassDesc',
    icon: Radio,
  },
  {
    to: '/failover',
    labelKey: 'nav.failover',
    descKey: 'nav.failoverDesc',
    icon: GitBranch,
  },
  {
    to: '/logs',
    labelKey: 'nav.logs',
    descKey: 'nav.logsDesc',
    icon: ScrollText,
  },
  {
    to: '/settings',
    labelKey: 'nav.settings',
    descKey: 'nav.settingsDesc',
    icon: Settings,
  },
]
