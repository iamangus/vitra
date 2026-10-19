<script>
  import { createEventDispatcher } from "svelte";
  export let title = "";
  export let description = "";
  export let busy = false;
  const dispatch = createEventDispatcher();
  function mount(node) {
    const previous = document.activeElement;
    node.showModal();
    return {
      destroy() {
        node.close();
        if (previous instanceof HTMLElement && previous.isConnected)
          previous.focus();
      },
    };
  }
  function outside(event) {
    const bounds = event.currentTarget.getBoundingClientRect();
    if (
      !busy &&
      (event.clientX < bounds.left ||
        event.clientX > bounds.right ||
        event.clientY < bounds.top ||
        event.clientY > bounds.bottom)
    )
      dispatch("cancel");
  }
</script>

<dialog
  class="dialog"
  aria-labelledby="dialog-title"
  aria-describedby={description ? "dialog-description" : undefined}
  use:mount
  on:cancel|preventDefault={() => {
    if (!busy) dispatch("cancel");
  }}
  on:click={outside}
>
  <h2 id="dialog-title">{title}</h2>
  {#if description}<p id="dialog-description" class="muted">
      {description}
    </p>{/if}
  <slot />
  <div class="dialog-actions"><slot name="actions" /></div>
</dialog>

<style>
  .dialog {
    width: min(460px, calc(100vw - 32px));
    max-height: calc(100dvh - 40px);
    overflow: auto;
    margin: auto;
    border: 1px solid var(--border-strong);
    border-radius: 16px;
    padding: 28px;
    color: var(--color);
    background: var(--bg-elevated);
    box-shadow: var(--shadow-lg);
  }
  .dialog::backdrop {
    background: var(--overlay-bg);
    backdrop-filter: blur(3px);
  }
  h2 {
    font-size: 21px;
    letter-spacing: -0.5px;
    margin: 0 0 8px;
    font-weight: 650;
  }
  p {
    font-size: 14px;
    line-height: 1.65;
    margin: 0 0 22px;
  }
  .dialog-actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 24px;
  }
  @media (max-width: 480px) {
    .dialog {
      padding: 22px;
    }
  }
</style>
