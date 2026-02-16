import { createRouter, createWebHashHistory } from 'vue-router'

const routes = [
  {
    path: '/',
    redirect: '/dashboard',
  },
  {
    path: '/mode-picker',
    name: 'ModePicker',
    component: () => import('../views/ModePicker.vue'),
  },
  {
    path: '/dashboard',
    name: 'Dashboard',
    component: () => import('../views/Dashboard.vue'),
  },
  {
    path: '/orchestrator',
    name: 'Orchestrator',
    component: () => import('../views/Orchestrator.vue'),
  },
  {
    path: '/remote',
    name: 'RemoteConnect',
    component: () => import('../views/RemoteConnect.vue'),
  },
  {
    path: '/settings',
    name: 'Settings',
    component: () => import('../views/Settings.vue'),
  },
]

export const router = createRouter({
  history: createWebHashHistory(),
  routes,
})
