import { api } from '../api.js'

let sysInfo = $state(null)
let dockerInfo = $state(null)
let loaded = $state(false)

async function load() {
  const [sysRes, dockerRes] = await Promise.all([
    api.systemInfo(),
    api.dockerInfo(),
  ])
  if (!sysRes.error) sysInfo = sysRes.data
  if (!dockerRes.error) dockerInfo = dockerRes.data
  else dockerInfo = null
  loaded = true
}

export const statusBar = {
  get sysInfo() { return sysInfo },
  get dockerInfo() { return dockerInfo },
  get loaded() { return loaded },
  load,
}
