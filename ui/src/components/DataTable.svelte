<script>
  let { columns = [], rows = [], emptyMessage = 'No data.' } = $props()
</script>

{#if rows.length === 0}
  <p class="text-text-secondary text-sm py-4">{emptyMessage}</p>
{:else}
  <div class="overflow-x-auto">
    <table class="w-full text-sm">
      <thead>
        <tr class="border-b border-border">
          {#each columns as col}
            <th class="text-left text-xs font-medium text-text-secondary uppercase tracking-wider py-2 px-3">{col.label}</th>
          {/each}
        </tr>
      </thead>
      <tbody class="divide-y divide-border-muted">
        {#each rows as row}
          <tr class="hover:bg-surface-1 transition-colors">
            {#each columns as col}
              <td class="py-2 px-3 text-text-primary">
                {#if col.render}
                  {@html col.render(row)}
                {:else}
                  {row[col.key] ?? ''}
                {/if}
              </td>
            {/each}
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
{/if}
