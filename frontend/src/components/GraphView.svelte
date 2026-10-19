<script>
  import { createEventDispatcher } from "svelte";
  import GraphCanvas from "./GraphCanvas.svelte";
  import Icon from "./Icon.svelte";
  import EmptyState from "./EmptyState.svelte";
  const dispatch = createEventDispatcher();
  let canvas;
  let nodes = [];
  let links = [];
  let selected = null;
  let query = "";
  let type = "all";
  let showList = false;
  let labels = false;
  let loading = true;
  let error = "";
  let canvasError = "";
  $: types = [...new Set(nodes.map((node) => node.type))].sort();
  $: matching = nodes.filter(
    (node) =>
      (type === "all" || node.type === type) &&
      (node.title + " " + node.id).toLowerCase().includes(query.toLowerCase()),
  );
  $: connections = selected
    ? links.filter(
        (link) =>
          link.sourceId === selected.id || link.targetId === selected.id,
      ).length
    : 0;

  function received(event) {
    nodes = event.detail.nodes;
    links = event.detail.links;
    loading = false;
    error = "";
    if (selected)
      selected = nodes.find((node) => node.id === selected.id) || null;
    canvas?.filterByType(type);
  }
  function select(node) {
    canvas?.selectNode(node.id);
    selected = node;
    showList = false;
    query = "";
  }
  function changeType(value) {
    type = value;
    canvas?.filterByType(type);
    if (selected && type !== "all" && selected.type !== type) {
      selected = null;
      canvas?.clearSelection();
    }
  }
  function retry() {
    error = "";
    loading = true;
    canvas?.retry();
  }
</script>

<div class="graph-page">
  <div class="graph-heading">
    <div>
      <div class="eyebrow">Follow an idea</div>
      <h1>Your knowledge, connected.</h1>
      <p>Explore the relationships between your notes.</p>
    </div>
    <span class="graph-info"
      >{nodes.length} note{nodes.length === 1 ? "" : "s"} · {links.length} link{links.length ===
      1
        ? ""
        : "s"}</span
    >
  </div>
  <div class="graph-tools">
    <div class="graph-search">
      <Icon name="search" size={17} /><input
        aria-label="Find a graph note"
        placeholder="Find a note in the graph…"
        bind:value={query}
      /><button
        class="icon-button"
        aria-label={showList ? "Hide note list" : "Browse graph notes"}
        aria-expanded={showList || Boolean(query)}
        on:click={() => (showList = !showList)}
        ><Icon name="outline" size={17} /></button
      >
      {#if query || showList}<div class="node-results" aria-label="Graph notes">
          {#each matching.slice(0, 20) as node}<button
              on:click={() => select(node)}
              ><Icon name="note" size={15} /><span
                >{node.title}<small>{node.type}</small></span
              ><Icon name="chevron" size={13} /></button
            >{:else}<p>No matching notes.</p>{/each}{#if matching.length > 20}<p
            >
              Showing 20 of {matching.length}. Type to narrow your search.
            </p>{/if}
        </div>{/if}
    </div>
    <div class="graph-buttons">
      <button
        class="button"
        title="Fit to view"
        disabled={!nodes.length || Boolean(canvasError)}
        on:click={() => canvas?.fit()}
        ><Icon name="fit" size={15} />Fit view</button
      ><button
        class="button"
        aria-label="Toggle always-visible node labels"
        aria-pressed={labels}
        disabled={Boolean(canvasError)}
        on:click={() => (labels = canvas?.labels())}
        ><Icon name="outline" size={15} />Labels</button
      >
    </div>
  </div>
  <div class="graph-legend" aria-label="Filter graph by note type">
    <button aria-pressed={type === "all"} on:click={() => changeType("all")}
      >All types</button
    >{#each types as item}<button
        aria-pressed={type === item}
        on:click={() => changeType(item)}>{item}</button
      >{/each}
  </div>
  <div class="graph-stage">
    <GraphCanvas
      bind:this={canvas}
      on:data={received}
      on:select={(event) => (selected = event.detail)}
      on:labels={(event) => (labels = event.detail)}
      on:loaderror={(event) => {
        error = event.detail;
        loading = false;
      }}
      on:canvaserror={(event) => {
        canvasError = event.detail;
        showList = true;
      }}
    />
    {#if loading}<div class="graph-overlay" aria-busy="true">
        <Icon name="graph" size={30} />
        <p>Finding the connections…</p>
      </div>
    {:else if error}<div class="graph-overlay">
        <EmptyState
          icon="warning"
          title="The graph could not load"
          message={error}
          compact
          ><button class="button" on:click={retry}>Try again</button
          ></EmptyState
        >
      </div>
    {:else if !nodes.length}<div class="graph-overlay">
        <EmptyState
          icon="graph"
          title="Ideas are better connected"
          message="Create a few notes and link them with WikiLinks or Markdown links. Their connections will appear here."
          compact
        />
      </div>
    {:else if canvasError}<div class="canvas-warning" role="status">
        {canvasError}
      </div>{/if}
    {#if selected}<aside class="node-inspector" aria-label="Selected note">
        <div class="inspector-heading">
          <span class="eyebrow">Selected note</span><button
            class="icon-button"
            aria-label="Close selected note"
            on:click={() => {
              selected = null;
              canvas?.clearSelection();
            }}><Icon name="close" size={15} /></button
          >
        </div>
        <span class="tag-chip">{selected.type}</span>
        <h2>{selected.title}</h2>
        <p class="node-path">{selected.id}</p>
        <p class="connection-count">
          <Icon name="link" size={14} />{connections} connection{connections ===
          1
            ? ""
            : "s"}
        </p>
        <button
          class="button primary"
          on:click={() => dispatch("navigate", selected.id)}
          >Open note<Icon name="arrow" size={15} /></button
        >
      </aside>{/if}
    {#if nodes.length && !selected && !error}<div class="graph-hint">
        Select a node to explore · Drag to rotate · Scroll to zoom
      </div>{/if}
  </div>
</div>

<style>
  .graph-page {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    padding: 28px 32px 20px;
  }
  .graph-heading {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 14px;
    margin-bottom: 22px;
  }
  h1 {
    font-size: 27px;
    font-weight: 600;
    letter-spacing: -0.9px;
    line-height: 1.25;
    margin: 10px 0;
  }
  .graph-heading p {
    color: var(--color-muted);
    font-size: 12px;
    margin: 0;
  }
  .graph-info {
    color: var(--color-muted);
    font-size: 11px;
    white-space: nowrap;
    margin-bottom: 2px;
  }
  .graph-tools {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .graph-search {
    display: flex;
    align-items: center;
    gap: 9px;
    position: relative;
    max-width: 390px;
    flex: 1;
    border: 1px solid var(--border-strong);
    border-radius: 7px;
    background: var(--bg-elevated);
    padding: 2px 5px 2px 12px;
    color: var(--color-muted);
  }
  .graph-search input {
    border: 0;
    background: none;
    min-width: 0;
    flex: 1;
    padding: 8px 0;
    font-size: 12px;
    outline: 0;
  }
  .graph-search:focus-within {
    outline: 2px solid var(--primary);
    outline-offset: 2px;
  }
  .graph-buttons {
    display: flex;
    gap: 6px;
    margin-left: auto;
  }
  .graph-buttons button {
    font-size: 11px;
    min-height: 36px;
    padding: 7px 10px;
  }
  .node-results {
    position: absolute;
    top: calc(100% + 7px);
    left: 0;
    right: 0;
    z-index: 30;
    max-height: 330px;
    overflow-y: auto;
    padding: 6px;
    border: 1px solid var(--border-strong);
    border-radius: 9px;
    background: var(--bg-elevated);
    box-shadow: var(--shadow-lg);
  }
  .node-results button {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 10px;
    text-align: left;
    background: none;
    border: 0;
    border-radius: 5px;
  }
  .node-results button:hover {
    background: var(--hover-bg);
  }
  .node-results button > span {
    min-width: 0;
    flex: 1;
    font-size: 12px;
    overflow-wrap: anywhere;
    color: var(--color);
  }
  .node-results small {
    display: block;
    font-size: 10px;
    margin-top: 3px;
    color: var(--color-muted);
  }
  .node-results p {
    font-size: 11px;
    color: var(--color-muted);
    padding: 8px;
  }
  .graph-legend {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin: 14px 0;
  }
  .graph-legend button {
    border: 1px solid transparent;
    background: none;
    color: var(--color-muted);
    border-radius: 5px;
    font-size: 10px;
    padding: 5px 8px;
  }
  .graph-legend button[aria-pressed="true"] {
    background: var(--primary-soft);
    color: var(--active-color);
    border-color: var(--border-color);
  }
  .graph-stage {
    position: relative;
    flex: 1;
    min-height: 160px;
    overflow: hidden;
    border: 1px solid var(--border-color);
    border-radius: 12px;
  }
  .graph-overlay {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    background: var(--bg);
    color: var(--color-muted);
    text-align: center;
    font-size: 12px;
  }
  .node-inspector {
    position: absolute;
    top: 16px;
    right: 16px;
    width: 240px;
    max-width: calc(100% - 32px);
    border: 1px solid var(--border-strong);
    background: var(--bg-elevated);
    box-shadow: var(--shadow-md);
    padding: 18px;
    border-radius: 10px;
    max-height: calc(100% - 32px);
    overflow-y: auto;
  }
  .inspector-heading {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin: -5px -6px 12px 0;
  }
  h2 {
    font-size: 18px;
    line-height: 1.5;
    font-weight: 550;
    letter-spacing: -0.4px;
    margin: 12px 0 6px;
    overflow-wrap: anywhere;
  }
  .node-path {
    font-size: 11px;
    color: var(--color-muted);
    overflow-wrap: anywhere;
  }
  .connection-count {
    display: flex;
    gap: 6px;
    align-items: center;
    font-size: 11px;
    color: var(--color-muted);
    margin: 18px 0;
  }
  .node-inspector > button {
    width: 100%;
  }
  .graph-hint {
    position: absolute;
    bottom: 14px;
    left: 16px;
    font-size: 10px;
    color: var(--color-muted);
    padding: 5px 8px;
    border-radius: 4px;
    background: var(--bg);
    max-width: calc(100% - 32px);
  }
  .canvas-warning {
    position: absolute;
    top: 16px;
    left: 16px;
    right: 16px;
    padding: 16px;
    color: var(--warning);
    background: var(--warning-bg);
    border-radius: 8px;
    font-size: 12px;
  }
  @media (max-width: 768px) {
    .graph-page {
      padding: 24px 16px 16px;
    }
    .graph-heading {
      flex-wrap: wrap;
      margin-bottom: 20px;
    }
    h1 {
      font-size: 24px;
    }
    .graph-tools {
      flex-wrap: wrap;
    }
    .graph-search {
      flex-basis: 100%;
      max-width: none;
    }
    .graph-search input {
      font-size: 16px;
    }
    .graph-buttons {
      margin-left: 0;
    }
    .graph-buttons button,
    .graph-legend button {
      min-height: 44px;
    }
    .graph-legend {
      margin: 10px 0;
    }
    .node-inspector {
      top: auto;
      bottom: 12px;
      left: 12px;
      right: 12px;
      width: auto;
      max-width: none;
      max-height: calc(100% - 24px);
      padding: 16px;
    }
    .graph-hint {
      font-size: 10px;
    }
  }
</style>
