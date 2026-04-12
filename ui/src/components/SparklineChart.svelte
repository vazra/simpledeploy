<script>
  let { data = [], color = '#3b82f6', width = 80, height = 24 } = $props()

  const points = $derived(() => {
    if (!data || data.length < 2) return null

    const valid = data.map((v, i) => ({ v, i })).filter(p => p.v != null)
    if (valid.length < 2) return null

    const minV = Math.min(...valid.map(p => p.v))
    const maxV = Math.max(...valid.map(p => p.v))
    const rangeV = maxV - minV || 1
    const pad = 2

    const toX = (i) => (i / (data.length - 1)) * width
    const toY = (v) => pad + (1 - (v - minV) / rangeV) * (height - pad * 2)

    const line = valid.map(p => `${toX(p.i).toFixed(2)},${toY(p.v).toFixed(2)}`).join(' ')

    const first = valid[0]
    const last = valid[valid.length - 1]
    const fill = [
      `${toX(first.i).toFixed(2)},${height}`,
      ...valid.map(p => `${toX(p.i).toFixed(2)},${toY(p.v).toFixed(2)}`),
      `${toX(last.i).toFixed(2)},${height}`
    ].join(' ')

    return { line, fill }
  })
</script>

{#if points()}
  <svg {width} {height} viewBox="0 0 {width} {height}" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
    <polygon points={points().fill} fill={color} fill-opacity="0.1" />
    <polyline points={points().line} stroke={color} stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
  </svg>
{/if}
