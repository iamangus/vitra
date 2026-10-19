<script>
  import { createEventDispatcher, onMount } from "svelte";
  import { search } from "../lib/api.js";
  import { subscribeToLiveUpdates } from "../lib/live.js";
  import Icon from "./Icon.svelte";
  import EmptyState from "./EmptyState.svelte";
  import HighlightedText from "./HighlightedText.svelte";
  export let query = "";
  const dispatch = createEventDispatcher();
  let inputValue = query;
  let mode = "fulltext";
  let results = [];
  let loading = false;
  let error = "";
  let input;
  let generation = 0;
  let controller;
  let reloadTimer;
  $: inputValue = query;
  $: if (query !== undefined) performSearch();

  async function performSearch() {
    const id = ++generation;
    controller?.abort();
    controller = new AbortController();
    const signal = controller.signal;
    error = "";
    if (!query.trim()) {
      results = [];
      loading = false;
      return;
    }
    loading = true;
    try {
      const response =
        mode === "semantic"
          ? await search.semantic(query, 12, signal)
          : await search.query(query, signal);
      if (id === generation) results = response || [];
    } catch (cause) {
      if (id === generation && cause.name !== "AbortError") {
        error =
          mode === "semantic"
            ? "Semantic search is unavailable right now. You can still find exact words with full-text search."
            : "Your search could not finish. Try again when your connection is restored.";
        results = [];
      }
    } finally {
      if (id === generation) loading = false;
    }
  }

  function submit() {
    query = inputValue.trim();
    dispatch("query", query);
    window.history.replaceState(
      {},
      "",
      "/search" + (query ? "?q=" + encodeURIComponent(query) : ""),
    );
  }
  function switchMode(value) {
    if (mode !== value) {
      mode = value;
      performSearch();
    }
  }
  function excerpt(result) {
    return (
      result.snippet ||
      (result.chunk || "")
        .replace(/^Note:.*?\| File:.*?\| Section:.*?\|\s*/, "")
        .slice(0, 260)
    );
  }

  onMount(() => {
    input?.focus();
    const unsubscribe = subscribeToLiveUpdates((event) => {
      if (event.search && query) {
        clearTimeout(reloadTimer);
        reloadTimer = setTimeout(performSearch, 250);
      }
    });
    return () => {
      generation++;
      controller?.abort();
      clearTimeout(reloadTimer);
      unsubscribe();
    };
  });
</script>

<div class="page">
  <div class="page-inner search-page">
    <div class="eyebrow">Find a connection</div>
    <h1>What are you looking for?</h1>
    <p class="page-description">
      Find the exact words, or explore the ideas behind them.
    </p>
    <form class="search-form" on:submit|preventDefault={submit}>
      <Icon name="search" size={21} /><input
        id="global-search-input"
        aria-label="Search notes"
        bind:this={input}
        bind:value={inputValue}
        maxlength="512"
        autocomplete="off"
        placeholder="Search your knowledge…"
      /><button class="button primary" type="submit">Search</button>
    </form>
    <div class="search-options">
      <div class="segmented" role="group" aria-label="Search mode">
        <button
          aria-pressed={mode === "fulltext"}
          on:click={() => switchMode("fulltext")}>Full-text</button
        ><button
          aria-pressed={mode === "semantic"}
          on:click={() => switchMode("semantic")}>Semantic</button
        >
      </div>
      <span
        >{mode === "fulltext"
          ? "Match words in your notes"
          : "Find related meaning and ideas"}</span
      >
    </div>
    {#if error}<div class="search-error" role="alert">
        <Icon name="warning" />
        <div>
          <strong>Search needs a moment</strong>
          <p>{error}</p>
          <button class="button" on:click={performSearch}>Try again</button
          >{#if mode === "semantic"}<button
              class="button quiet"
              on:click={() => switchMode("fulltext")}>Use full-text</button
            >{/if}
        </div>
      </div>
    {:else if loading}<div
        class="results-skeleton"
        aria-busy="true"
        aria-label="Searching notes"
      >
        {#each [1, 2, 3] as item}<div>
            <div class="skeleton" style="width:45%;height:16px"></div>
            <div class="skeleton" style="width:90%;height:12px"></div>
            <div class="skeleton" style="width:70%;height:12px"></div>
          </div>{/each}
      </div>
    {:else if query && results.length}<div
        class="results-heading"
        role="status"
      >
        {results.length} result{results.length === 1 ? "" : "s"} for
        <strong>“{query}”</strong>
      </div>
      <div class="search-results">
        {#each results as result}<a
            href={"/note/" + encodeURIComponent(result.path)}
            on:click|preventDefault={() => dispatch("navigate", result.path)}
            ><span class="result-icon"><Icon name="note" size={19} /></span>
            <div class="result-body">
              <div class="result-path">
                {result.path.includes("/")
                  ? result.path.slice(0, result.path.lastIndexOf("/"))
                  : "Vault root"}{#if result.type}<span>
                    · {result.type}</span
                  >{/if}
              </div>
              <h2><HighlightedText text={result.title} {query} /></h2>
              {#if excerpt(result)}<p>
                  <HighlightedText text={excerpt(result)} {query} />
                </p>{/if}{#if result.heading}<span class="result-section"
                  >{result.heading}</span
                >{/if}
            </div>
            <span class="result-arrow"><Icon name="arrow" size={17} /></span></a
          >{/each}
      </div>
    {:else if query}<EmptyState
        icon="search"
        title="No notes found, yet"
        message="Try a shorter phrase, a different word, or switch search modes. Your next connection might be one search away."
        compact
      />
    {:else}<div class="search-guide">
        <Icon name="search" size={26} />
        <h2>A name, a phrase, an idea.</h2>
        <p>
          Full-text finds the words you remember.<br />Semantic search helps
          when you remember the meaning.
        </p>
        <div>
          <span class="tag-chip">Project names</span><span class="tag-chip"
            >Technical concepts</span
          ><span class="tag-chip">A line you remember</span>
        </div>
      </div>{/if}
  </div>
</div>

<style>
  .search-page {
    max-width: 820px;
  }
  .search-form {
    display: flex;
    align-items: center;
    gap: 13px;
    padding: 8px 9px 8px 17px;
    border: 1px solid var(--border-strong);
    border-radius: 10px;
    background: var(--bg-elevated);
    box-shadow: var(--shadow-sm);
  }
  .search-form:focus-within {
    border-color: var(--primary);
    outline: 2px solid var(--primary-soft);
  }
  .search-form > :first-child {
    color: var(--color-muted);
  }
  input {
    min-width: 0;
    flex: 1;
    padding: 10px 0;
    border: 0;
    background: none;
    font-size: 15px;
    outline: 0;
  }
  input::placeholder {
    color: var(--color-faint);
  }
  .search-options {
    display: flex;
    align-items: center;
    gap: 15px;
    margin: 16px 0 32px;
  }
  .search-options > span {
    color: var(--color-muted);
    font-size: 11px;
  }
  .results-heading {
    font-size: 12px;
    color: var(--color-muted);
    margin-bottom: 8px;
  }
  .results-heading strong {
    color: var(--color);
    font-weight: 500;
  }
  .search-results a {
    display: flex;
    gap: 16px;
    padding: 24px 8px;
    border-bottom: 1px solid var(--border-color);
    text-decoration: none;
    color: var(--color);
  }
  .search-results a:hover {
    background: var(--hover-bg);
    border-radius: 8px;
  }
  .result-icon {
    display: grid;
    place-items: center;
    width: 36px;
    height: 40px;
    border: 1px solid var(--border-color);
    border-radius: 7px;
    background: var(--bg-elevated);
    color: var(--color-muted);
    flex-shrink: 0;
  }
  .result-body {
    min-width: 0;
    flex: 1;
  }
  .result-path,
  .result-section {
    display: block;
    font-size: 11px;
    color: var(--color-muted);
    overflow-wrap: anywhere;
  }
  h2 {
    font-size: 17px;
    font-weight: 550;
    letter-spacing: -0.4px;
    margin: 7px 0 9px;
    overflow-wrap: anywhere;
  }
  .result-body p {
    font-size: 13px;
    line-height: 1.8;
    color: var(--color-muted);
    margin: 0 0 8px;
    overflow-wrap: anywhere;
  }
  .result-arrow {
    padding-top: 20px;
    color: var(--color-muted);
  }
  .search-error {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    padding: 20px;
    background: var(--warning-bg);
    color: var(--warning);
    border-radius: 10px;
  }
  .search-error p {
    font-size: 13px;
    line-height: 1.7;
  }
  .search-guide {
    text-align: center;
    color: var(--color-muted);
    padding: 60px 16px;
  }
  .search-guide > div {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: 8px;
    margin-top: 22px;
  }
  .search-guide h2 {
    color: var(--color);
    font-size: 20px;
    margin-top: 20px;
  }
  .search-guide p {
    font-size: 13px;
    line-height: 1.8;
  }
  .results-skeleton > div {
    display: grid;
    gap: 14px;
    padding: 30px 12px;
    border-bottom: 1px solid var(--border-color);
  }
  @media (max-width: 768px) {
    input {
      font-size: 16px;
    }
    .search-options {
      align-items: flex-start;
      flex-direction: column;
      gap: 10px;
    }
    .search-form {
      padding-left: 12px;
      gap: 9px;
    }
    .result-icon {
      display: none;
    }
    .search-results a {
      gap: 8px;
      padding: 22px 0;
    }
    .result-arrow {
      display: none;
    }
  }
</style>
