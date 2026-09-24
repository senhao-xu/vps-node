import {
  Globe2,
  LayoutDashboard,
  Server,
  Settings,
  Users,
  Waypoints,
  type LucideIcon,
} from 'lucide-vue-next'

export type NavigationItem = {
  to: string
  label: string
  keywords: string
  icon: LucideIcon
  exact?: boolean
}

export const navigationItems: NavigationItem[] = [
  {
    to: '/',
    label: '仪表盘',
    keywords: 'dashboard yibiaopan home',
    icon: LayoutDashboard,
    exact: true,
  },
  { to: '/users', label: '用户', keywords: 'users yonghu', icon: Users },
  { to: '/servers', label: '服务器', keywords: 'servers fuwuqi', icon: Server },
  { to: '/nodes', label: '节点', keywords: 'nodes jiedian', icon: Waypoints },
  { to: '/visits', label: '访问记录', keywords: 'visits fangwen jilu sites', icon: Globe2 },
  { to: '/settings', label: '设置', keywords: 'settings shezhi', icon: Settings },
]
