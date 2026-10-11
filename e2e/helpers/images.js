// Image refs for containers the tests start directly (outside SimpleDeploy,
// which rewrites its own deploys). With E2E_USE_MIRROR=1 (CI), Docker Hub
// refs go through the GHCR mirror, same naming as internal/mirror. Every
// ref used here must be listed in fixtures/helper-images.yml so the mirror
// workflow copies it.
export function mirrored(ref) {
  if (process.env.E2E_USE_MIRROR !== '1') return ref;
  let prefix = process.env.SIMPLEDEPLOY_IMAGE_MIRROR_PREFIX || 'ghcr.io/vazra/simpledeploy-mirror/';
  if (!prefix.endsWith('/')) prefix += '/';
  const first = ref.split('/')[0];
  const hasHost = ref.includes('/') && (first.includes('.') || first.includes(':') || first === 'localhost');
  if (hasHost && first !== 'docker.io') return ref;
  return prefix + ref.replace(/^docker\.io\//, '').replace(/^library\//, '');
}
