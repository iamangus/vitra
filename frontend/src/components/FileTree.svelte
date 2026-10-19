<script>
  import { createEventDispatcher } from "svelte";
  import Icon from "./Icon.svelte";
  export let nodes = [];
  export let activePath = "";
  export let depth = 0;
  export let expanded = new Set();
  const dispatch = createEventDispatcher();
  let dragOverPath = "";

  function toggle(event, path) {
    if (event.currentTarget.open === expanded.has(path)) return;
    const next = new Set(expanded);
    if (event.currentTarget.open) next.add(path);
    else next.delete(path);
    expanded = next;
  }

  function startDrag(event, node) {
    event.dataTransfer.setData("text/plain", node.Path);
    event.dataTransfer.effectAllowed = "move";
    dispatch("dragstate", true);
  }

  function dragOver(event, node) {
    if (!event.dataTransfer.types.includes("text/plain")) return;
    event.preventDefault();
    event.stopPropagation();
    event.dataTransfer.dropEffect = "move";
    dragOverPath = node.Path;
  }

  function drop(event, node) {
    event.preventDefault();
    event.stopPropagation();
    dragOverPath = "";
    dispatch("dragstate", false);
    const source = event.dataTransfer.getData("text/plain");
    if (!source || source === node.Path || node.Path.startsWith(source + "/"))
      return;
    const name = source.split("/").pop();
    const newPath = node.Path + "/" + name;
    if (source !== newPath)
      dispatch("move", {
        oldPath: source,
        newPath,
        isDir:
          event.dataTransfer.getData("application/x-vitra-directory") ===
          "true",
      });
  }

  function begin(event, node) {
    event.dataTransfer.setData(
      "application/x-vitra-directory",
      String(node.IsDir),
    );
    startDrag(event, node);
  }
</script>

{#each nodes as node (String(node.IsDir) + ":" + node.Path)}
  <div class="tree-item" class:selected={node.Path === activePath}>
    {#if node.IsDir}
      <details
        open={expanded.has(node.Path)}
        on:toggle={(event) => toggle(event, node.Path)}
        class:drop-target={dragOverPath === node.Path}
        on:dragover={(event) => dragOver(event, node)}
        on:dragleave={() => (dragOverPath = "")}
        on:drop={(event) => drop(event, node)}
      >
        <summary
          style={"padding-left:" + (depth * 14 + 6) + "px"}
          data-path={node.Path}
          data-isdir="true"
          draggable="true"
          on:dragstart={(event) => begin(event, node)}
          on:dragend={() => dispatch("dragstate", false)}
        >
          <span class="chevron"><Icon name="chevron" size={12} /></span><Icon
            name="folder"
            size={16}
          /><span class="name">{node.Name}</span>
        </summary>
        <div>
          <svelte:self
            nodes={node.Children || []}
            {activePath}
            depth={depth + 1}
            bind:expanded
            on:navigate
            on:actions
            on:move
            on:dragstate
          />
        </div>
      </details>
    {:else}
      <a
        href={"/note/" + encodeURIComponent(node.Path)}
        style={"padding-left:" + (depth * 14 + 20) + "px"}
        data-path={node.Path}
        data-isdir="false"
        aria-current={node.Path === activePath ? "page" : undefined}
        draggable="true"
        on:dragstart={(event) => begin(event, node)}
        on:dragend={() => dispatch("dragstate", false)}
        on:click|preventDefault={() => dispatch("navigate", node.Path)}
        ><Icon name="note" size={15} /><span class="name" title={node.Name}
          >{node.Name}</span
        ></a
      >
    {/if}
    <button
      class="row-actions"
      aria-label={"Actions for " + node.Name}
      aria-haspopup="menu"
      on:click={(event) =>
        dispatch("actions", {
          path: node.Path,
          isDir: node.IsDir,
          anchor: event.currentTarget,
        })}><Icon name="more" size={16} /></button
    >
  </div>
{/each}

<style>
  .tree-item {
    position: relative;
    margin: 2px 0;
  }
  summary,
  a {
    display: flex;
    align-items: center;
    gap: 7px;
    min-height: 34px;
    padding: 6px 34px 6px 6px;
    border-radius: 6px;
    color: var(--color-muted);
    font-size: 12px;
    text-decoration: none;
    cursor: pointer;
    list-style: none;
  }
  summary::-webkit-details-marker {
    display: none;
  }
  summary:hover,
  a:hover {
    color: var(--color);
    background: var(--hover-bg);
  }
  .name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }
  .selected > a {
    background: var(--active-bg);
    color: var(--active-color);
    font-weight: 500;
  }
  .row-actions {
    position: absolute;
    right: 2px;
    top: 1px;
    display: grid;
    place-items: center;
    width: 30px;
    height: 32px;
    padding: 0;
    border: 0;
    color: var(--color-muted);
    background: transparent;
  }
  .row-actions:hover {
    color: var(--color);
    background: var(--hover-bg);
  }
  .chevron {
    display: flex;
    transition: transform 0.15s;
  }
  details[open] > summary .chevron {
    transform: rotate(90deg);
  }
  .drop-target > summary {
    background: var(--primary-soft);
    outline: 1px solid var(--primary);
  }
  @media (max-width: 768px) {
    summary,
    a {
      min-height: 44px;
      padding-right: 44px;
    }
    .row-actions {
      width: 42px;
      height: 42px;
    }
  }
</style>
