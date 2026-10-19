<script>
  import Icon from "./Icon.svelte";
  export let icon = "note";
  export let title = "";
  export let message = "";
  export let compact = false;
</script>

<div class="empty-state" class:compact>
  <span class="empty-symbol"><Icon name={icon} size={26} /></span>
  <h2>{title}</h2>
  <p>{message}</p>
  <div class="empty-actions"><slot /></div>
</div>

<style>
  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    text-align: center;
    padding: 64px 24px;
    max-width: 500px;
    margin: auto;
  }
  .empty-symbol {
    display: grid;
    place-items: center;
    width: 56px;
    height: 56px;
    border: 1px solid var(--border-color);
    border-radius: 16px;
    color: var(--primary);
    background: var(--bg-elevated);
    margin-bottom: 22px;
  }
  h2 {
    font-size: 24px;
    font-weight: 600;
    letter-spacing: -0.7px;
    margin: 0 0 12px;
  }
  p {
    font-size: 14px;
    line-height: 1.75;
    color: var(--color-muted);
    max-width: 360px;
    margin: 0;
  }
  .empty-actions {
    display: flex;
    justify-content: center;
    flex-wrap: wrap;
    gap: 10px;
    margin-top: 24px;
  }
  .compact {
    padding: 32px 18px;
  }
  .compact h2 {
    font-size: 18px;
  }
</style>
