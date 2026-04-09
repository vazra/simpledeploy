<script>
  import { onMount, onDestroy } from 'svelte'
  import { Chart, registerables } from 'chart.js'
  import 'chartjs-adapter-date-fns'
  import { effectiveTheme } from '../lib/stores/theme.js'

  Chart.register(...registerables)

  let { data = [], label = '', color = '#58a6ff', unit = '' } = $props()
  let canvas
  let chart

  function getGridColor(theme) {
    return theme === 'light' ? '#e5e7eb' : '#21262d'
  }

  function getTickColor(theme) {
    return theme === 'light' ? '#656d76' : '#8b949e'
  }

  function createChart(theme) {
    if (chart) chart.destroy()
    chart = new Chart(canvas, {
      type: 'line',
      data: {
        datasets: [{
          label,
          data,
          borderColor: color,
          backgroundColor: color + '20',
          fill: true,
          tension: 0.3,
          pointRadius: 0,
          borderWidth: 1.5,
        }]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        scales: {
          x: {
            type: 'time',
            time: { unit: 'minute' },
            grid: { color: getGridColor(theme) },
            ticks: { color: getTickColor(theme), font: { size: 10 } }
          },
          y: {
            beginAtZero: true,
            grid: { color: getGridColor(theme) },
            ticks: {
              color: getTickColor(theme),
              font: { size: 10 },
              callback: (v) => v + unit
            }
          }
        },
        plugins: { legend: { display: false } }
      }
    })
  }

  let currentTheme = 'dark'
  let mounted = false

  onMount(() => {
    mounted = true
    const unsub = effectiveTheme.subscribe((t) => {
      currentTheme = t
    })
    return () => {
      unsub()
      if (chart) { chart.destroy(); chart = null }
    }
  })

  $effect(() => {
    if (mounted && canvas && data) {
      createChart(currentTheme)
    }
  })
</script>

<div class="bg-surface-2 border border-border rounded-lg p-4">
  <h4 class="text-xs font-medium text-text-secondary mb-3">{label}</h4>
  <div class="h-44 relative">
    <canvas bind:this={canvas}></canvas>
  </div>
</div>
