<script>
  import { createEventDispatcher } from 'svelte';

  const dispatch = createEventDispatcher();

  export let nodes = [];
  export let activePath = '';
  export let depth = 0;

  function handleClick(node) {
    if (!node.IsDir) {
      dispatch('navigate', node.Path);
    }
  }
</script>

{#each nodes as node}
  <div class="file-tree-item">
    {#if node.IsDir}
      <details class="group" open={node.IsOpen}>
        <summary
          class="flex items-center gap-1.5 py-1 rounded-md cursor-pointer select-none list-none"
          style="padding-left: {depth * 12}px;"
          data-path={node.Path}
          data-isdir="true"
        >
          <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="opacity-60 arrow-icon transition-transform">
            <polyline points="9 18 15 12 9 6"/>
          </svg>
          <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="opacity-70">
            <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13c0 1.1.9 2 2 2Z"/>
          </svg>
          <span class="text-sm truncate">{node.Name}</span>
        </summary>
        <div class="pl-3">
          <svelte:self nodes={node.Children} {activePath} depth={depth + 1} on:navigate />
        </div>
      </details>
    {:else}
      <a
        href="/note/{node.Path}"
        class="flex items-center gap-1.5 py-1 rounded-md text-sm truncate"
        class:active={node.Path === activePath}
        style="padding-left: {depth * 12 + 18}px; color: inherit; text-decoration: none;"
        data-path={node.Path}
        data-isdir="false"
        on:click|preventDefault={() => handleClick(node)}
      >
        <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="opacity-50">
          <path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z"/>
          <polyline points="14 2 14 8 20 8"/>
        </svg>
        <span class="text-sm truncate">{node.Name}</span>
      </a>
    {/if}
  </div>
{/each}

<style>
  .file-tree-item {
    margin-bottom: 1px;
  }

  .file-tree-item summary {
    display: flex;
    align-items: center;
    gap: 0.375rem;
    padding: 0.25rem 0;
    border-radius: 0.375rem;
    cursor: pointer;
    user-select: none;
    list-style: none;
  }

  .file-tree-item a {
    display: flex;
    align-items: center;
    gap: 0.375rem;
    padding: 0.25rem 0;
    border-radius: 0.375rem;
    font-size: 0.875rem;
    color: inherit;
    text-decoration: none;
  }

  .file-tree-item a:hover,
  .file-tree-item summary:hover {
    background: var(--hover-bg);
  }

  .file-tree-item a.active {
    background: var(--active-bg);
    color: var(--active-color);
  }

  details[open] > summary .arrow-icon {
    transform: rotate(90deg);
  }

  details > summary {
    list-style: none;
  }

  details > summary::-webkit-details-marker {
    display: none;
  }
</style>
