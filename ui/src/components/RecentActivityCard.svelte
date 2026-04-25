<script>
  import { onMount, onDestroy } from 'svelte';
  import { api } from '../lib/api.js';
  import ActivityRow from './ActivityRow.svelte';

  let entries = $state([]);
  let loading = $state(true);
  let timer;

  async function refresh() {
    try {
      const res = await api.listRecentActivity(8);
      entries = res?.entries || [];
    } catch (e) {
      // soft-fail; keep previous entries
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    refresh();
    timer = setInterval(refresh, 30_000);
  });

  onDestroy(() => clearInterval(timer));
</script>

<div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
  <div class="flex items-center justify-between mb-3">
    <h3 class="text-sm font-semibold text-text-primary">Recent Activity</h3>
    <a href="#/system?tab=audit" class="text-xs text-accent hover:underline">View all</a>
  </div>

  {#if loading && entries.length === 0}
    <p class="text-xs text-text-secondary">Loading…</p>
  {:else if entries.length === 0}
    <p class="text-xs text-text-secondary italic">No activity yet.</p>
  {:else}
    <div class="flex flex-col gap-2">
      {#each entries as e (e.id)}
        <ActivityRow entry={e} compact showAppColumn />
      {/each}
    </div>
  {/if}
</div>
