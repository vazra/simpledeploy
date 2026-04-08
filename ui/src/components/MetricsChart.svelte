<script>
  import { onMount, onDestroy } from 'svelte'
  import { Chart, registerables } from 'chart.js'
  import 'chartjs-adapter-date-fns'

  Chart.register(...registerables)

  let { data = [], label = '', color = '#58a6ff', unit = '' } = $props()
  let canvas
  let chart

  onMount(() => {
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
            grid: { color: '#21262d' },
            ticks: { color: '#8b949e', font: { size: 10 } }
          },
          y: {
            beginAtZero: true,
            grid: { color: '#21262d' },
            ticks: {
              color: '#8b949e',
              font: { size: 10 },
              callback: (v) => v + unit
            }
          }
        },
        plugins: {
          legend: { display: false },
        }
      }
    })
  })

  onDestroy(() => { if (chart) chart.destroy() })
</script>

<div class="chart-container">
  <h4>{label}</h4>
  <div class="chart-wrapper">
    <canvas bind:this={canvas}></canvas>
  </div>
</div>

<style>
  .chart-container {
    background: #1c1f26;
    border: 1px solid #2d3139;
    border-radius: 8px;
    padding: 1rem;
  }
  h4 {
    margin: 0 0 0.75rem;
    font-size: 0.85rem;
    font-weight: 500;
    color: #8b949e;
  }
  .chart-wrapper {
    height: 180px;
    position: relative;
  }
</style>
