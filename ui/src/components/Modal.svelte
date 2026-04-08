<script>
  let { title = 'Confirm', message = '', onConfirm = () => {}, onCancel = () => {} } = $props()
</script>

<div class="overlay" onclick={onCancel}>
  <div class="modal" onclick={(e) => e.stopPropagation()}>
    <h3>{title}</h3>
    <p>{message}</p>
    <div class="actions">
      <button class="cancel" onclick={onCancel}>Cancel</button>
      <button class="confirm" onclick={onConfirm}>Confirm</button>
    </div>
  </div>
</div>

<style>
  .overlay {
    position: fixed; inset: 0; background: rgba(0,0,0,0.6);
    display: flex; align-items: center; justify-content: center; z-index: 100;
  }
  .modal {
    background: #1c1f26; border: 1px solid #2d3139; border-radius: 8px;
    padding: 1.5rem; min-width: 320px; max-width: 480px;
  }
  h3 { margin: 0 0 0.5rem; color: #e1e4e8; font-size: 1rem; }
  p { color: #8b949e; font-size: 0.85rem; margin: 0 0 1.25rem; }
  .actions { display: flex; justify-content: flex-end; gap: 0.5rem; }
  .cancel {
    padding: 0.4rem 0.8rem; background: none; border: 1px solid #30363d;
    border-radius: 4px; color: #8b949e; cursor: pointer; font-size: 0.8rem;
  }
  .cancel:hover { color: #e1e4e8; border-color: #8b949e; }
  .confirm {
    padding: 0.4rem 0.8rem; background: #da3633; border: none;
    border-radius: 4px; color: #fff; cursor: pointer; font-size: 0.8rem;
  }
  .confirm:hover { background: #f85149; }
</style>
