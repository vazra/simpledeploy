<script>
  let { onClose = () => {} } = $props()

  const REPO = 'https://github.com/vazra/simpledeploy'

  const links = [
    {
      href: `${REPO}/issues/new?template=bug_report.yml`,
      title: 'Report a bug',
      desc: 'Found something broken? Open a GitHub issue with steps to reproduce.',
      icon: '<path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />',
    },
    {
      href: `${REPO}/issues/new?template=feature_request.yml`,
      title: 'Suggest a feature',
      desc: 'Have an idea? Tell us what would make SimpleDeploy better.',
      icon: '<path stroke-linecap="round" stroke-linejoin="round" d="M12 18v-5.25m0 0a6.01 6.01 0 001.5-.189m-1.5.189a6.01 6.01 0 01-1.5-.189m3.75 7.478a12.06 12.06 0 01-4.5 0m3.75 2.383a14.406 14.406 0 01-3 0M14.25 18v-.192c0-.983.658-1.823 1.508-2.316a7.5 7.5 0 10-7.517 0c.85.493 1.509 1.333 1.509 2.316V18" />',
    },
    {
      href: `${REPO}/discussions`,
      title: 'Feedback & discussions',
      desc: 'Share feedback, ask questions, or chat with the community.',
      icon: '<path stroke-linecap="round" stroke-linejoin="round" d="M2.25 12.76c0 1.6 1.123 2.994 2.707 3.227 1.068.157 2.148.279 3.238.364.466.037.893.281 1.153.671L12 21l2.652-3.978c.26-.39.687-.634 1.153-.67 1.09-.086 2.17-.208 3.238-.365 1.584-.233 2.707-1.626 2.707-3.228V6.741c0-1.602-1.123-2.995-2.707-3.228A48.394 48.394 0 0012 3c-2.392 0-4.744.175-7.043.513C3.373 3.746 2.25 5.14 2.25 6.741v6.018z" />',
    },
    {
      href: `${REPO}/blob/main/CONTRIBUTING.md`,
      title: 'Contribute a fix',
      desc: 'SimpleDeploy is open source. Fork, fix, and send a PR, your name in the changelog.',
      icon: '<path stroke-linecap="round" stroke-linejoin="round" d="M6 13.5V3.75m0 9.75a1.5 1.5 0 010 3m0-3a1.5 1.5 0 000 3m0 3.75V16.5m12-3V3.75m0 9.75a1.5 1.5 0 010 3m0-3a1.5 1.5 0 000 3m0 3.75V16.5m-6-13.5V20.25m0-17.25a1.5 1.5 0 010 3m0-3a1.5 1.5 0 000 3" />',
    },
    {
      href: 'https://vazra.github.io/simpledeploy/',
      title: 'Documentation',
      desc: 'Guides, references, and how-tos.',
      icon: '<path stroke-linecap="round" stroke-linejoin="round" d="M12 6.042A8.967 8.967 0 006 3.75c-1.052 0-2.062.18-3 .512v14.25A8.987 8.987 0 016 18c2.305 0 4.408.867 6 2.292m0-14.25a8.966 8.966 0 016-2.292c1.052 0 2.062.18 3 .512v14.25A8.987 8.987 0 0018 18a8.967 8.967 0 00-6 2.292m0-14.25v14.25" />',
    },
    {
      href: REPO,
      title: 'Star on GitHub',
      desc: 'Support the project, helps others discover it.',
      icon: '<path stroke-linecap="round" stroke-linejoin="round" d="M11.48 3.499a.562.562 0 011.04 0l2.125 5.111a.563.563 0 00.475.345l5.518.442c.499.04.701.663.32.988l-4.204 3.602a.563.563 0 00-.182.557l1.285 5.385a.562.562 0 01-.84.61l-4.725-2.885a.563.563 0 00-.586 0L6.982 20.54a.562.562 0 01-.84-.61l1.285-5.386a.562.562 0 00-.182-.557l-4.204-3.602a.562.562 0 01.32-.988l5.518-.442a.563.563 0 00.475-.345L11.48 3.5z" />',
    },
  ]

  function onKeydown(e) {
    if (e.key === 'Escape') onClose()
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="fixed inset-0 z-50 flex items-center justify-center" role="dialog" aria-modal="true" aria-label="Help menu">
  <button class="absolute inset-0 bg-black/50 backdrop-blur-sm" onclick={onClose} aria-label="Close"></button>
  <div class="relative bg-surface-2 border border-border/50 rounded-2xl p-6 w-[28rem] max-w-[92vw] shadow-2xl animate-scale-in">
    <div class="flex items-start justify-between mb-1">
      <h3 class="text-lg font-semibold text-text-primary tracking-tight">Help & feedback</h3>
      <button onclick={onClose} class="text-text-muted hover:text-text-primary -mt-1" aria-label="Close">
        <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12"/></svg>
      </button>
    </div>
    <p class="text-xs text-text-muted mb-4">SimpleDeploy is open source. All feedback flows through GitHub, you can also send a PR to fix or improve anything.</p>
    <div class="flex flex-col gap-1.5">
      {#each links as l}
        <a
          href={l.href}
          target="_blank"
          rel="noopener noreferrer"
          class="flex items-start gap-3 px-3 py-2.5 rounded-lg text-text-secondary hover:text-text-primary hover:bg-surface-3/50 transition-colors group"
        >
          <span class="shrink-0 mt-0.5 text-accent">
            <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">{@html l.icon}</svg>
          </span>
          <span class="flex flex-col min-w-0">
            <span class="text-sm font-medium text-text-primary">{l.title}</span>
            <span class="text-xs text-text-muted">{l.desc}</span>
          </span>
          <span class="ml-auto shrink-0 self-center text-text-muted group-hover:text-text-primary">
            <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M13.5 6H5.25A2.25 2.25 0 003 8.25v10.5A2.25 2.25 0 005.25 21h10.5A2.25 2.25 0 0018 18.75V10.5m-10.5 6L21 3m0 0h-5.25M21 3v5.25"/></svg>
          </span>
        </a>
      {/each}
    </div>
  </div>
</div>
