<script>
  import Router from 'svelte-spa-router'
  import Toast from './components/Toast.svelte'
  import Login from './routes/Login.svelte'
  import Dashboard from './routes/Dashboard.svelte'
  import AppDetail from './routes/AppDetail.svelte'
  import Backups from './routes/Backups.svelte'
  import Alerts from './routes/Alerts.svelte'
  import Users from './routes/Users.svelte'
  import Registries from './routes/Registries.svelte'
  import Docker from './routes/Docker.svelte'
  import System from './routes/System.svelte'

  const routes = {
    '/login': Login,
    '/': Dashboard,
    '/apps/:slug': AppDetail,
    '/backups': Backups,
    '/alerts': Alerts,
    '/users': Users,
    '/registries': Registries,
    '/docker': Docker,
    '/system': System,
  }
</script>

<Router {routes} />
<Toast />
