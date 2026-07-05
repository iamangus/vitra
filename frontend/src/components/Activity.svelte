<script>
  import { onMount } from 'svelte';
  import { activity } from '../lib/api.js';
  import { subscribeToLiveUpdates } from '../lib/live.js';

  let entries = $state([]);
  let loading = $state(true);
  let error = $state(null);
  let reloadTimeout = null;

  async function loadFeed() {
    loading = true;
    error = null;
    try {
      entries = await activity.list({ limit: '100' });
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  function scheduleReload() {
    clearTimeout(reloadTimeout);
    reloadTimeout = setTimeout(loadFeed, 250);
  }

  onMount(() => {
    loadFeed();
    const unsub = subscribeToLiveUpdates((event) => {
      if (event.activity) scheduleReload();
    });
    return () => {
      clearTimeout(reloadTimeout);
      unsub();
    };
  });
</script>

<div class="activity-panel">
  {#if loading}
    <div class="loading">Loading activity...</div>
  {:else if error}
    <div class="error">{error}</div>
  {:else if entries.length === 0}
    <div class="empty">
      <p>No activity recorded.</p>
      <p class="hint">Add <code>log.md</code> files with ISO date headings and bullet entries to see activity here.</p>
    </div>
  {:else}
    <div class="entries">
      {#each entries as entry}
        <div class="entry">
          <div class="entry-meta">
            <span class="entry-date">{entry.date}</span>
            <span class="entry-source" title={entry.source}>{entry.source_path || entry.source}</span>
          </div>
          <div class="entry-text">{entry.text}</div>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .activity-panel {
    flex: 1;
    overflow-y: auto;
    padding: 0.75rem;
  }

  .loading, .error, .empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 100%;
    color: var(--color-muted);
    font-size: 0.8125rem;
    padding: 2rem;
    text-align: center;
    gap: 0.5rem;
  }

  .error {
    color: var(--accent-missing);
  }

  .hint {
    font-size: 0.75rem;
    opacity: 0.7;
  }

  .hint code {
    background: var(--hover-bg);
    padding: 0.125rem 0.375rem;
    border-radius: var(--radius-sm);
    font-size: 0.75rem;
  }

  .entries {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .entry {
    padding: 0.5rem 0.625rem;
    border-radius: var(--radius-sm);
    background: var(--bg);
    border: 1px solid var(--border-color);
  }

  .entry-meta {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    margin-bottom: 0.25rem;
  }

  .entry-date {
    font-size: 0.6875rem;
    font-weight: 600;
    color: var(--primary);
    font-family: var(--font-mono);
  }

  .entry-source {
    font-size: 0.625rem;
    color: var(--color-faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 12rem;
  }

  .entry-text {
    font-size: 0.8125rem;
    color: var(--color);
    line-height: 1.5;
  }
</style>
