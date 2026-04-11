<script>
  import { onMount, onDestroy } from 'svelte'
  import { Chart, registerables } from 'chart.js'
  import 'chartjs-adapter-date-fns'
  import { effectiveTheme } from '../lib/stores/theme.js'

  Chart.register(...registerables)

  let { data = [], label = '', color = '#58a6ff', unit = '', subtitle = '', tooltipFormat = null, interval = 60 } = $props()
  let canvas
  let chart

  function getTimeUnit(iv) {
    if (iv <= 60) return 'minute'
    if (iv <= 3600) return 'hour'
    return 'day'
  }

  const gapShadePlugin = {
    id: 'gapShade',
    beforeDatasetsDraw(chart) {
      const meta = chart.getDatasetMeta(0)
      const ctx = chart.ctx
      const yScale = chart.scales.y
      if (!meta.data || meta.data.length < 2) return
      const dataset = chart.data.datasets[0].data
      ctx.save()
      ctx.fillStyle = chart.options.scales.x.grid.color || 'rgba(128,128,128,0.1)'
      for (let i = 0; i < dataset.length; i++) {
        if (dataset[i].y === null || dataset[i].y === undefined) {
          const prevPt = i > 0 ? meta.data[i - 1] : null
          const nextPt = i < meta.data.length - 1 ? meta.data[i + 1] : null
          if (prevPt && nextPt) {
            ctx.fillRect(prevPt.x, yScale.top, nextPt.x - prevPt.x, yScale.bottom - yScale.top)
          }
        }
      }
      ctx.restore()
    }
  }

  function getGridColor(theme) {
    return theme === 'light' ? '#f0f0f0' : '#1a1a1a'
  }

  function getTickColor(theme) {
    return theme === 'light' ? '#737373' : '#666666'
  }

  function createChart(theme) {
    if (chart) chart.destroy()
    chart = new Chart(canvas, {
      type: 'line',
      data: {
        datasets: [{
          label,
          data: [...data],
          borderColor: color,
          backgroundColor: color + '20',
          fill: true,
          tension: 0.3,
          pointRadius: 0,
          borderWidth: 1.5,
          spanGaps: false,
        }]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        scales: {
          x: {
            type: 'time',
            time: { unit: getTimeUnit(interval) },
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
        plugins: {
          legend: { display: false },
          tooltip: {
            callbacks: {
              label: tooltipFormat
                ? (ctx) => tooltipFormat(ctx.dataIndex, ctx.parsed.y)
                : (ctx) => `${label}: ${ctx.parsed.y.toFixed(1)}${unit}`
            }
          }
        }
      },
      plugins: [gapShadePlugin]
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

<div class="bg-surface-2 rounded-xl p-4 shadow-sm border border-border/50">
  <div class="flex items-baseline gap-2 mb-4">
    <h4 class="text-xs font-medium text-text-secondary">{label}</h4>
    {#if subtitle}<span class="text-xs text-text-muted">{subtitle}</span>{/if}
  </div>
  <div class="h-44 relative">
    <canvas bind:this={canvas}></canvas>
  </div>
</div>
