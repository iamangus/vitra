<script>
  import { createEventDispatcher } from 'svelte';
  import { search } from '../lib/api.js';

  const dispatch = createEventDispatcher();

  export let query = '';

  let results = [];
  let loading = false;

  $: if (query) {
    performSearch();
  }

  async function performSearch() {
    if (!query) {
      results = [];
      return;
    }
    loading = true;
    try {
      results = await search.query(query);
    } catch (e) {
      console.error('Search failed:', e);
      results = [];
    } finally {
      loading = false;
    }
  }

  function handleSubmit(e) {
    e.preventDefault();
    const formData = new FormData(e.target);
    query = formData.get('q');
    window.history.pushState({}, '', `/search?q=${encodeURIComponent(query)}`);
  }
</script>

<div class="search-page">
  <div class="search-container">
    <h1 class="search-title">Search</h1>

    <form on:submit={handleSubmit} class="search-form">
      <div class="search-input-wrap">
        <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="search-icon">
          <circle cx="11" cy="11" r="8"/>
          <path d="m21 21-4.3-4.3"/>
        </svg>
        <input
          type="text"
          name="q"
          value={query}
          placeholder="Search notes..."
          class="search-input"
        >
        <button type="submit" class="search-btn">Search</button>
      </div>
    </form>

    {#if query}
      <p class="search-meta">
        {results.length} result{results.length !== 1 ? 's' : ''} for "{query}"
      </p>

      {#if loading}
        <div class="search-loading">
          <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="spin">
            <path d="M21 12a9 9 0 1 1-6.219-8.56"/>
          </svg>
          <span>Searching...</span>
        </div>
      {:else if results.length > 0}
        <div class="search-results">
          {#each results as result}
            <a
              href="/note/{result.path}"
              class="search-result"
              on:click|preventDefault={() => dispatch('navigate', result.path)}
            >
              <div class="result-icon">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z"/>
                  <polyline points="14 2 14 8 20 8"/>
                </svg>
              </div>
              <div class="result-body">
                <span class="result-title">{result.title}</span>
                {#if result.path.includes('/')}
                  <span class="result-path">{result.path}</span>
                {/if}
              </div>
              <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="result-arrow">
                <path d="m9 18 6-6-6-6"/>
              </svg>
            </a>
          {/each}
        </div>
      {:else}
        <div class="search-empty">
          <svg xmlns="http://www.w3.org/2000/svg" width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1" stroke-linecap="round" stroke-linejoin="round" class="empty-icon">
            <circle cx="11" cy="11" r="8"/>
            <path d="m21 21-4.3-4.3"/>
          </svg>
          <p>No results found</p>
        </div>
      {/if}
    {/if}
  </div>
</div>

<style>
  .search-page {
    flex: 1;
    overflow-y: auto;
    padding: 2rem 1.5rem;
  }

  .search-container {
    max-width: 640px;
    margin: 0 auto;
  }

  .search-title {
    font-size: 1.5rem;
    font-weight: 700;
    margin: 0 0 1.5rem;
    letter-spacing: -0.02em;
  }

  .search-form {
    margin-bottom: 1.5rem;
  }

  .search-input-wrap {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    background: var(--bg-elevated);
    border: 1px solid var(--border-color);
    border-radius: var(--radius);
    padding: 0.5rem 0.75rem;
    transition: border-color 0.15s, box-shadow 0.15s;
  }

  .search-input-wrap:focus-within {
    border-color: var(--primary);
    box-shadow: 0 0 0 3px var(--primary-soft);
  }

  .search-icon {
    color: var(--color-faint);
    flex-shrink: 0;
  }

  .search-input {
    flex: 1;
    border: none;
    outline: none;
    background: transparent;
    color: var(--color);
    font-size: 0.9375rem;
    padding: 0.25rem 0;
  }

  .search-input::placeholder {
    color: var(--color-faint);
  }

  .search-btn {
    padding: 0.375rem 0.875rem;
    border-radius: var(--radius-sm);
    font-size: 0.8125rem;
    font-weight: 600;
    background: var(--primary);
    color: var(--primary-color);
    border: none;
    cursor: pointer;
    transition: background 0.15s;
    flex-shrink: 0;
  }

  .search-btn:hover {
    background: var(--primary-hover);
  }

  .search-meta {
    font-size: 0.8125rem;
    color: var(--color-muted);
    margin: 0 0 1rem;
  }

  .search-loading {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    color: var(--color-faint);
    font-size: 0.875rem;
  }

  .spin {
    animation: spin 1s linear infinite;
  }

  @keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }

  .search-results {
    display: flex;
    flex-direction: column;
    gap: 0.375rem;
  }

  .search-result {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.875rem 1rem;
    border-radius: var(--radius);
    background: var(--bg-elevated);
    border: 1px solid var(--border-color);
    text-decoration: none;
    color: inherit;
    transition: all 0.15s;
  }

  .search-result:hover {
    border-color: var(--border-strong);
    background: var(--hover-bg);
  }

  .result-icon {
    color: var(--color-faint);
    flex-shrink: 0;
  }

  .result-body {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.125rem;
  }

  .result-title {
    font-weight: 500;
    font-size: 0.9375rem;
    color: var(--color);
  }

  .result-path {
    font-size: 0.75rem;
    color: var(--color-faint);
  }

  .result-arrow {
    color: var(--color-faint);
    flex-shrink: 0;
    transition: transform 0.15s;
  }

  .search-result:hover .result-arrow {
    transform: translateX(2px);
    color: var(--color-muted);
  }

  .search-empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.75rem;
    padding: 3rem 1rem;
    text-align: center;
  }

  .search-empty .empty-icon {
    color: var(--color-faint);
    opacity: 0.5;
  }

  .search-empty p {
    margin: 0;
    color: var(--color-muted);
    font-size: 0.9375rem;
  }

  /* Mobile */
  @media (max-width: 768px) {
    .search-page {
      padding: 1.25rem;
    }

    .search-title {
      font-size: 1.25rem;
    }
  }
</style>
