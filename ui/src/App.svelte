<script>
  import Router from 'svelte-spa-router'
  import Login from './routes/Login.svelte'
  import Dashboard from './routes/Dashboard.svelte'
  import AppDetail from './routes/AppDetail.svelte'

  const routes = {
    '/login': Login,
    '/': Dashboard,
    '/apps/:slug': AppDetail,
  }
</script>

<Router {routes} />
