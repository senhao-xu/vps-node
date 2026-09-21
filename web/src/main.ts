import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { router } from './router'
import { setUnauthorizedHandler } from './api/http'
import { useThemeStore } from './stores/theme'
import './styles/tokens.css'
import './styles/base.css'

const app = createApp(App)

app.use(createPinia())

useThemeStore().apply()

setUnauthorizedHandler(() => {
  const current = router.currentRoute.value
  if (current.name === 'login') return
  void router.push({ name: 'login', query: { redirect: current.fullPath } })
})

app.use(router)
app.mount('#app')
