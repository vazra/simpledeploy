<script>
  import { onMount, onDestroy } from 'svelte'
  import { Chart, registerables } from 'chart.js'
  import 'chartjs-adapter-date-fns'
  import { effectiveTheme } from '../lib/stores/theme.js'

  Chart.register(...registerables)

  // Single dataset: pass `data` + `color`. Multi dataset: pass `datasets` [{label, data, color}].
  // formatValue: optional (value) => string for tooltip display (e.g. show bytes alongside %)
  let { data = [], datasets = null, label = '', color = '#58a6ff', unit = '', subtitle = '', tooltipFormat = null, formatValue = null, interval = 60 } = $props()
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
      // Shade gaps on the first dataset (Total or the single series)
      const meta = chart.getDatasetMeta(0)
      const ctx = chart.ctx
      const yScale = chart.scales.y
      if (!meta.data || meta.data.length < 2) return
      const points = chart.data.datasets[0].data
      ctx.save()
      ctx.fillStyle = chart.options.scales.x.grid.color || 'rgba(128,128,128,0.1)'
      for (let i = 0; i < points.length; i++) {
        if (points[i].y === null || points[i].y === undefined) {
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

  function buildDatasets() {
    if (datasets && datasets.length > 0) {
      return datasets.map((ds, i) => ({
        label: ds.label || `Container ${i + 1}`,
        data: [...ds.data],
        borderColor: ds.color,
        backgroundColor: ds.color + '20',
        fill: datasets.length === 1,
        tension: 0.3,
        pointRadius: 0,
        borderWidth: 1.5,
        spanGaps: false,
      }))
    }
    return [{
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
  }

  function createChart(theme) {
    if (chart) chart.destroy()
    const ds = buildDatasets()
    const multiSeries = ds.length > 1
    chart = new Chart(canvas, {
      type: 'line',
      data: { datasets: ds },
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
          legend: {
            display: multiSeries,
            labels: { color: getTickColor(theme), font: { size: 10 }, boxWidth: 12 }
          },
          tooltip: {
            callbacks: {
              label(ctx) {
                if (tooltipFormat) return tooltipFormat(ctx.dataIndex, ctx.parsed.y)
                const raw = ctx.raw
                const v = ctx.parsed.y?.toFixed(1) ?? '?'
                if (formatValue && raw?.extra != null) {
                  return `${ctx.dataset.label}: ${v}${unit} (${formatValue(raw.extra)})`
                }
                return `${ctx.dataset.label}: ${v}${unit}`
              }
            }
          }
        }
      },
      plugins: [gapShadePlugin]
    })
  }

  let currentTheme = 'dark'
  let mounted = false

  function updateChart() {
    if (!mounted || !canvas) return
    if (!chart) {
      createChart(currentTheme)
      return
    }
    chart.data.datasets = buildDatasets()
    chart.options.scales.x.grid.color = getGridColor(currentTheme)
    chart.options.scales.x.ticks.color = getTickColor(currentTheme)
    chart.options.scales.y.grid.color = getGridColor(currentTheme)
    chart.options.scales.y.ticks.color = getTickColor(currentTheme)
    if (chart.options.plugins.legend) {
      chart.options.plugins.legend.labels.color = getTickColor(currentTheme)
    }
    chart.update('none')
  }

  onMount(() => {
    mounted = true
    const unsub = effectiveTheme.subscribe((t) => {
      currentTheme = t
      updateChart()
    })
    return () => {
      unsub()
      if (chart) { chart.destroy(); chart = null }
    }
  })

  $effect(() => {
    if (mounted && canvas && (data || datasets)) {
      updateChart()
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
