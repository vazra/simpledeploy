<script>
  import Router from 'svelte-spa-router'
  import Login from './routes/Login.svelte'
  import Dashboard from './routes/Dashboard.svelte'
  import AppDetail from './routes/AppDetail.svelte'
  import Backups from './routes/Backups.svelte'
  import Alerts from './routes/Alerts.svelte'
  import Users from './routes/Users.svelte'

  const routes = {
    '/login': Login,
    '/': Dashboard,
    '/apps/:slug': AppDetail,
    '/backups': Backups,
    '/alerts': Alerts,
    '/users': Users,
  }
</script>

<Router {routes} />
