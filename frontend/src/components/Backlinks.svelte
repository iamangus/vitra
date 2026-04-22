<script>
  import { createEventDispatcher } from 'svelte';

  const dispatch = createEventDispatcher();

  export let backlinks = [];
</script>

{#if backlinks.length > 0}
  <div class="backlinks-section">
    <div class="backlinks-header">
      <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M9 14 4 9l5-5"/>
        <path d="M4 9h10.5a5.5 5.5 0 0 1 5.5 5.5v0a5.5 5.5 0 0 1-5.5 5.5H11"/>
      </svg>
      <span>Linked from</span>
    </div>
    <div class="backlinks-list">
      {#each backlinks as link}
        <a
          href="/note/{link.path}"
          class="backlink-item"
          on:click|preventDefault={() => dispatch('navigate', link.path)}
        >
          <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z"/>
            <polyline points="14 2 14 8 20 8"/>
          </svg>
          <span class="backlink-title">{link.title}</span>
        </a>
      {/each}
    </div>
  </div>
{/if}

<style>
  .backlinks-section {
    margin-top: 3rem;
    padding-top: 1.5rem;
    border-top: 1px solid var(--border-color);
  }

  .backlinks-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--color-faint);
    margin-bottom: 0.75rem;
  }

  .backlinks-header svg {
    opacity: 0.6;
  }

  .backlinks-list {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .backlink-item {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem 0.625rem;
    border-radius: var(--radius-sm);
    font-size: 0.875rem;
    color: var(--color-muted);
    text-decoration: none;
    transition: all 0.15s;
  }

  .backlink-item svg {
    opacity: 0.4;
    flex-shrink: 0;
  }

  .backlink-item:hover {
    background: var(--hover-bg);
    color: var(--color);
  }

  .backlink-title {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
