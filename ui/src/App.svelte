<script>
  import Router from 'svelte-spa-router'
  import Login from './routes/Login.svelte'
  import Dashboard from './routes/Dashboard.svelte'

  const routes = {
    '/login': Login,
    '/': Dashboard,
  }
</script>

<Router {routes} />
