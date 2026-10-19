<script>
  import { createEventDispatcher, onMount, tick } from "svelte";
  import { files, folders, notes, fileOps } from "../lib/api.js";
  import { subscribeToLiveUpdates } from "../lib/live.js";
  import { flushAffectedDrafts } from "../lib/drafts.js";
  import { focusScope } from "../lib/focus.js";
  import { theme } from "../stores/theme.js";
  import FileTree from "./FileTree.svelte";
  import Dialog from "./Dialog.svelte";
  import Icon from "./Icon.svelte";

  export let activePath = "";
  export let view = "home";
  export let sidebarOpen = true;
  export let mobile = false;
  const dispatch = createEventDispatcher();
  let treeData = [];
  let loading = true;
  let treeError = "";
  let expanded = new Set();
  let appearance = "system";
  let menu = null;
  let action = null;
  let fieldValue = "";
  let destination = "";
  let busy = false;
  let error = "";
  let reloadTimer;
  let requestId = 0;
  let dragging = false;
  let dragOverRoot = false;
  let sidebarElement;
  const navigation = [
    { id: "home", name: "Notes", icon: "note" },
    { id: "search", name: "Search", icon: "search" },
    { id: "graph", name: "Graph", icon: "graph" },
    { id: "activity", name: "Activity", icon: "activity" },
  ];
  try {
    appearance = localStorage.getItem("theme") || "system";
  } catch {}

  $: folderOptions = flatten(treeData).filter(
    (node) =>
      node.IsDir &&
      (!action?.isDir ||
        (node.Path !== action.path &&
          !node.Path.startsWith(action.path + "/"))),
  );
  $: actionTitle =
    action?.kind === "delete"
      ? `Delete ${action.isDir ? "folder" : "note"}?`
      : {
          note: "Create a note",
          folder: "Create a folder",
          rename: "Rename " + (action?.isDir ? "folder" : "note"),
          move: "Move " + (action?.isDir ? "folder" : "note"),
        }[action?.kind] || "";
  $: if (mobile && sidebarOpen && sidebarElement) trapDrawer(sidebarElement);
  $: expandActive(activePath);

  function expandActive(path) {
    const parts = path.split("/");
    expanded = new Set([
      ...expanded,
      ...parts
        .slice(0, -1)
        .map((_, index) => parts.slice(0, index + 1).join("/")),
    ]);
  }

  function flatten(nodes) {
    return nodes.flatMap((node) => [
      node,
      ...(node.Children ? flatten(node.Children) : []),
    ]);
  }
  function parent(path) {
    return path.includes("/") ? path.slice(0, path.lastIndexOf("/")) : "";
  }
  function entryPath(path, isDir) {
    return isDir ? path : path + ".md";
  }
  function join(folder, name) {
    return folder ? folder + "/" + name : name;
  }

  async function loadTree() {
    const id = ++requestId;
    try {
      const result = await files.list(activePath);
      if (id !== requestId) return;
      treeData = result || [];
      treeError = "";
      dispatch(
        "tree",
        flatten(treeData)
          .filter((node) => !node.IsDir)
          .map((node) => ({ path: node.Path, title: node.Name })),
      );
    } catch {
      if (id === requestId) treeError = "Your notes could not load.";
    } finally {
      if (id === requestId) loading = false;
    }
  }

  export async function revealFolder(path) {
    const next = new Set(expanded);
    const parts = path.split("/");
    for (let index = 1; index <= parts.length; index++)
      next.add(parts.slice(0, index).join("/"));
    expanded = next;
    await tick();
    const target = sidebarElement?.querySelector(
      `summary[data-path="${CSS.escape(path)}"]`,
    );
    target?.scrollIntoView({ block: "nearest" });
    target?.focus();
  }

  export function openCreate(kind = "note", folder = parent(activePath)) {
    menu = null;
    action = { kind, path: "", isDir: kind === "folder" };
    fieldValue = "";
    destination = folder;
    error = "";
  }

  function openAction(kind) {
    const selected = menu;
    menu = null;
    action = { kind, path: selected.path, isDir: selected.isDir };
    fieldValue = selected.path.split("/").pop();
    destination = parent(selected.path);
    error = "";
  }

  function showActions(event) {
    const { path, isDir, anchor } = event.detail;
    const rect = anchor.getBoundingClientRect();
    menu = {
      path,
      isDir,
      x: Math.min(rect.right, Math.max(12, window.innerWidth - 208)),
      y: Math.min(rect.bottom + 5, Math.max(12, window.innerHeight - 286)),
    };
  }

  function contextMenu(event) {
    const target = event.target.closest("[data-path]");
    if (!target) return;
    event.preventDefault();
    menu = {
      path: target.dataset.path,
      isDir: target.dataset.isdir === "true",
      x: Math.max(12, Math.min(event.clientX, window.innerWidth - 208)),
      y: Math.max(12, Math.min(event.clientY, window.innerHeight - 286)),
    };
  }

  function navigateAfterRename(oldPath, newPath, isDir) {
    if (!isDir && activePath === oldPath) dispatch("navigate", newPath);
    else if (isDir && activePath.startsWith(oldPath + "/"))
      dispatch("navigate", newPath + activePath.slice(oldPath.length));
  }

  async function move(oldPath, newPath, isDir) {
    await flushAffectedDrafts(oldPath);
    await fileOps.rename(entryPath(oldPath, isDir), entryPath(newPath, isDir));
    navigateAfterRename(oldPath, newPath, isDir);
    if (parent(newPath)) {
      const next = new Set(expanded);
      next.add(parent(newPath));
      expanded = next;
    }
    await loadTree();
  }

  async function submit() {
    if (busy) return;
    error = "";
    const name = fieldValue.trim();
    if (
      ["note", "folder", "rename"].includes(action.kind) &&
      (!name || name === "." || name === ".." || /[\\/\x00]/.test(name))
    ) {
      error = "Enter a name without slashes. Choose the folder separately.";
      return;
    }
    busy = true;
    try {
      if (action.kind === "note" || action.kind === "folder") {
        const path = join(destination, name);
        if (action.kind === "note") {
          await notes.create(path);
          dispatch("created", path);
        } else {
          await folders.create(path);
          expanded = new Set([...expanded, path]);
        }
      } else if (action.kind === "delete") {
        await flushAffectedDrafts(action.path);
        await fileOps.delete(entryPath(action.path, action.isDir));
        if (
          (!action.isDir && activePath === action.path) ||
          (action.isDir && activePath.startsWith(action.path + "/"))
        )
          dispatch("view", "home");
      } else {
        const newPath =
          action.kind === "move"
            ? join(destination, action.path.split("/").pop())
            : join(parent(action.path), name);
        if (newPath !== action.path)
          await move(action.path, newPath, action.isDir);
      }
      action = null;
      await loadTree();
    } catch (cause) {
      error =
        cause.status === 409
          ? "That destination already exists. Choose a different name or folder."
          : cause.message ||
            "The action could not finish. Your notes have not been discarded.";
    } finally {
      busy = false;
    }
  }

  async function handleMove(event) {
    const { oldPath, newPath, isDir } = event.detail;
    try {
      await move(oldPath, newPath, isDir);
    } catch (cause) {
      action = { kind: "move", path: oldPath, isDir };
      destination = parent(newPath);
      error = cause.message;
    }
  }

  async function rootDrop(event) {
    event.preventDefault();
    dragOverRoot = false;
    dragging = false;
    const source = event.dataTransfer.getData("text/plain");
    const isDir =
      event.dataTransfer.getData("application/x-vitra-directory") === "true";
    const node = flatten(treeData).find(
      (node) => node.Path === source && node.IsDir === isDir,
    );
    if (node && parent(source))
      await handleMove({
        detail: {
          oldPath: source,
          newPath: source.split("/").pop(),
          isDir: node.IsDir,
        },
      });
  }

  function menuKeyboard(event) {
    if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
    event.preventDefault();
    const items = [
      ...event.currentTarget.querySelectorAll('[role="menuitem"]'),
    ];
    const index = items.indexOf(document.activeElement);
    items[
      event.key === "Home"
        ? 0
        : event.key === "End"
          ? items.length - 1
          : (index + (event.key === "ArrowDown" ? 1 : -1) + items.length) %
            items.length
    ]?.focus();
  }

  let drawerCleanup;
  function trapDrawer(node) {
    if (drawerCleanup) return;
    const close = (event) => {
      if (event.key === "Escape" && !action && !menu) dispatch("toggle");
    };
    const trap = focusScope(node);
    node.addEventListener("keydown", close);
    drawerCleanup = () => {
      trap.destroy();
      node.removeEventListener("keydown", close);
      drawerCleanup = null;
    };
  }
  $: if ((!mobile || !sidebarOpen) && drawerCleanup) drawerCleanup();

  onMount(() => {
    loadTree();
    const unsubscribe = subscribeToLiveUpdates((event) => {
      if (event.tree || event.notes) {
        clearTimeout(reloadTimer);
        reloadTimer = setTimeout(loadTree, 250);
      }
    });
    return () => {
      requestId++;
      clearTimeout(reloadTimer);
      unsubscribe();
      drawerCleanup?.();
    };
  });
</script>

<svelte:window
  on:keydown={(event) => {
    if (event.key === "Escape") menu = null;
  }}
/>

<aside
  id="vault-navigation"
  class="sidebar"
  class:collapsed={!sidebarOpen}
  class:mobile
  aria-label="Vault navigation"
  inert={!sidebarOpen}
  bind:this={sidebarElement}
>
  <div class="brand-row">
    <button
      class="brand"
      aria-label="Vitra home"
      on:click={() => dispatch("view", "home")}
      ><svg
        width="25"
        height="27"
        viewBox="0 0 25 27"
        fill="none"
        aria-hidden="true"
        ><path
          d="M3 4l9.5 19L22 4 M7 4l5.5 11L18 4"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        /></svg
      ><span>Vitra</span></button
    >{#if mobile}<button
        class="icon-button"
        aria-label="Close navigation"
        on:click={() => dispatch("toggle")}><Icon name="close" /></button
      >{/if}
  </div>
  <div class="vault-label">Local knowledge vault</div>
  <button class="search-launcher" on:click={() => dispatch("view", "search")}
    ><Icon name="search" size={16} /><span>Find a note</span><kbd
      >⌘ / Ctrl K</kbd
    ></button
  >
  <button
    class="button primary create-button"
    on:click={() => openCreate("note")}
    ><Icon name="plus" size={17} />New note</button
  >
  <nav class="primary-navigation" aria-label="Workspace">
    {#each navigation as item}<button
        class:active={view === item.id ||
          (view === "note" && item.id === "home")}
        aria-current={view === item.id ||
        (view === "note" && item.id === "home")
          ? "page"
          : undefined}
        on:click={() => dispatch("view", item.id)}
        ><Icon name={item.icon} size={17} />{item.name}</button
      >{/each}
  </nav>
  <div class="tree-heading">
    <span class="eyebrow">Vault</span><button
      class="icon-button"
      aria-label="New folder"
      title="New folder"
      on:click={() => openCreate("folder", "")}
      ><Icon name="plus" size={15} /></button
    >
  </div>
  <div
    class="file-tree"
    on:contextmenu={contextMenu}
    role="navigation"
    aria-label="Notes"
  >
    {#if loading}<div
        class="tree-skeleton"
        aria-label="Loading notes"
        aria-busy="true"
      >
        {#each [75, 90, 60, 80] as width}<div
            class="skeleton"
            style={"width:" + width + "%"}
          ></div>{/each}
      </div>
    {:else if treeError}<p class="tree-error" role="alert">{treeError}</p>
      <button class="button" on:click={loadTree}>Try again</button>
    {:else if !treeData.length}<p class="tree-empty">
        A place for your first note.<br />Create one to get started.
      </p>
    {:else}<FileTree
        nodes={treeData}
        {activePath}
        bind:expanded
        on:navigate
        on:actions={showActions}
        on:move={handleMove}
        on:dragstate={(event) => (dragging = event.detail)}
      />{/if}
    {#if dragging}<div
        class="root-drop"
        class:over={dragOverRoot}
        role="presentation"
        on:dragover={(event) => {
          event.preventDefault();
          dragOverRoot = true;
        }}
        on:dragleave={() => (dragOverRoot = false)}
        on:drop={rootDrop}
      >
        Drop to move to vault root
      </div>{/if}
  </div>
  <div class="sidebar-footer">
    <Icon name="sun" size={16} /><label for="appearance">Appearance</label
    ><select
      id="appearance"
      aria-label="Appearance"
      bind:value={appearance}
      on:change={() => theme.set(appearance)}
      ><option value="system">System</option><option value="light">Light</option
      ><option value="dark">Dark</option></select
    >
  </div>
</aside>

{#if menu}
  <button
    class="menu-dismiss"
    aria-label="Close file actions"
    on:click={() => (menu = null)}
  ></button>
  <div
    class="file-menu"
    role="menu"
    aria-label="File actions"
    tabindex="-1"
    use:focusScope
    style={"left:" + menu.x + "px;top:" + menu.y + "px"}
    on:keydown={menuKeyboard}
  >
    <div class="menu-label">{menu.path.split("/").pop()}</div>
    {#if menu.isDir}<button
        role="menuitem"
        on:click={() => openCreate("note", menu.path)}
        ><Icon name="plus" size={16} />New note here</button
      >{/if}
    <button role="menuitem" on:click={() => openAction("rename")}
      ><Icon name="edit" size={16} />Rename</button
    >
    <button role="menuitem" on:click={() => openAction("move")}
      ><Icon name="move" size={16} />Move to…</button
    >
    <button
      role="menuitem"
      on:click={() => {
        files.download(entryPath(menu.path, menu.isDir));
        menu = null;
      }}><Icon name="download" size={16} />Download</button
    >
    <div class="menu-separator"></div>
    <button
      role="menuitem"
      class="delete-action"
      on:click={() => openAction("delete")}
      ><Icon name="trash" size={16} />Delete</button
    >
  </div>
{/if}

{#if action}
  <Dialog
    title={actionTitle}
    description={action.kind === "delete"
      ? action.isDir
        ? "This permanently deletes the folder and everything inside it. This cannot be undone."
        : "This permanently deletes the note. This cannot be undone."
      : "Keep your knowledge organized, one note at a time."}
    {busy}
    on:cancel={() => (action = null)}
  >
    <form id="file-action-form" on:submit|preventDefault={submit}>
      {#if action.kind === "delete"}<div class="delete-name">
          <Icon name={action.isDir ? "folder" : "note"} /><strong
            >{action.path}</strong
          >
        </div>
      {:else}
        {#if action.kind !== "move"}<label class="field-label" for="entry-name"
            >Name</label
          ><input
            class="field"
            id="entry-name"
            bind:value={fieldValue}
            disabled={busy}
            required
            autocomplete="off"
            placeholder={action.isDir ? "Folder name" : "Untitled note"}
          />{/if}
        {#if action.kind === "note" || action.kind === "folder" || action.kind === "move"}<label
            class="field-label"
            for="entry-folder"
            >{action.kind === "move" ? "Destination folder" : "Location"}</label
          ><select
            class="field"
            id="entry-folder"
            bind:value={destination}
            disabled={busy}
            ><option value="">Vault root</option
            >{#each folderOptions as folder}<option value={folder.Path}
                >{folder.Path}</option
              >{/each}</select
          >{:else}<p class="field-hint">
            Location: {parent(action.path) || "Vault root"}
          </p>{/if}
        {#if action.kind === "move"}<p class="field-hint">
            Moving: {action.path}
          </p>{/if}
      {/if}
      {#if error}<p class="inline-error" role="alert">{error}</p>{/if}
    </form>
    <svelte:fragment slot="actions"
      ><button
        class="button quiet"
        disabled={busy}
        on:click={() => (action = null)}>Cancel</button
      ><button
        class="button"
        class:primary={action.kind !== "delete"}
        class:danger={action.kind === "delete"}
        type="submit"
        form="file-action-form"
        disabled={busy}
        >{busy
          ? "Working…"
          : action.kind === "delete"
            ? "Delete permanently"
            : action.kind === "move"
              ? "Move"
              : action.kind === "rename"
                ? "Rename"
                : "Create"}</button
      ></svelte:fragment
    >
  </Dialog>
{/if}

<style>
  .sidebar {
    width: 256px;
    min-width: 256px;
    display: flex;
    flex-direction: column;
    padding: 25px 16px 12px;
    background: var(--sidebar-bg);
    border-right: 1px solid var(--border-color);
    height: 100dvh;
    overflow: hidden;
  }
  .sidebar.collapsed {
    display: none;
  }
  .brand-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 10px;
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 9px;
    border: 0;
    background: none;
    padding: 0;
    color: var(--color);
    font-size: 20px;
    font-weight: 650;
    letter-spacing: -0.8px;
  }
  .brand svg {
    color: var(--primary);
  }
  .vault-label {
    color: var(--color-muted);
    font-size: 11px;
    padding: 6px 10px 22px;
  }
  .search-launcher {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 10px;
    font-size: 12px;
    border: 1px solid var(--border-strong);
    color: var(--color-muted);
    background: var(--bg-elevated);
    min-height: 38px;
  }
  kbd {
    margin-left: auto;
    font: inherit;
    font-size: 9px;
    white-space: nowrap;
  }
  .create-button {
    margin-top: 10px;
    min-height: 38px;
  }
  .primary-navigation {
    display: flex;
    flex-direction: column;
    gap: 3px;
    margin: 24px 0 20px;
  }
  .primary-navigation button {
    display: flex;
    align-items: center;
    gap: 11px;
    min-height: 35px;
    text-align: left;
    border: 0;
    padding: 8px 11px;
    background: transparent;
    font-size: 12px;
    color: var(--color-muted);
  }
  .primary-navigation button:hover {
    background: var(--hover-bg);
    color: var(--color);
  }
  .primary-navigation button.active {
    background: var(--active-bg);
    color: var(--active-color);
    font-weight: 550;
  }
  .tree-heading {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding-left: 10px;
  }
  .file-tree {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    margin: 2px -4px 12px;
    padding: 0 4px;
  }
  .tree-empty {
    color: var(--color-muted);
    font-size: 12px;
    line-height: 1.8;
    padding: 6px 10px;
  }
  .tree-skeleton {
    display: grid;
    gap: 17px;
    padding: 12px 10px;
  }
  .tree-skeleton div {
    height: 13px;
  }
  .tree-error {
    color: var(--danger);
    font-size: 12px;
    padding: 4px 10px;
  }
  .sidebar-footer {
    display: flex;
    align-items: center;
    gap: 8px;
    border-top: 1px solid var(--border-color);
    padding: 13px 7px 0;
    color: var(--color-muted);
    font-size: 11px;
  }
  .sidebar-footer select {
    width: 80px;
    margin-left: auto;
    padding: 5px 0 5px 4px;
    color: var(--color);
    background: transparent;
    border: 0;
    font-size: 11px;
  }
  .menu-dismiss {
    position: fixed;
    inset: 0;
    border: 0;
    background: transparent;
    z-index: 400;
  }
  .file-menu {
    position: fixed;
    width: 196px;
    z-index: 410;
    background: var(--bg-elevated);
    border: 1px solid var(--border-strong);
    border-radius: 10px;
    box-shadow: var(--shadow-lg);
    padding: 6px;
  }
  .file-menu button {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 8px 10px;
    min-height: 36px;
    font-size: 12px;
    text-align: left;
    border: 0;
    background: transparent;
  }
  .file-menu button:hover {
    background: var(--hover-bg);
  }
  .menu-label {
    color: var(--color-muted);
    font-size: 11px;
    padding: 5px 10px 8px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .file-menu .delete-action {
    color: var(--danger);
  }
  .menu-separator {
    height: 1px;
    background: var(--border-color);
    margin: 5px 0;
  }
  .delete-name {
    display: flex;
    gap: 10px;
    padding: 14px;
    background: var(--danger-bg);
    color: var(--danger);
    border-radius: 7px;
    overflow-wrap: anywhere;
  }
  .delete-name strong {
    min-width: 0;
    font-size: 13px;
    font-weight: 550;
  }
  .root-drop {
    padding: 18px 8px;
    margin-top: 15px;
    border: 1px dashed var(--border-strong);
    border-radius: 8px;
    font-size: 11px;
    text-align: center;
    color: var(--color-muted);
  }
  .root-drop.over {
    border-color: var(--primary);
    color: var(--primary);
    background: var(--primary-soft);
  }
  @media (max-width: 768px) {
    .sidebar {
      position: fixed;
      inset: 0 auto 0 0;
      width: min(296px, 86vw);
      min-width: 0;
      z-index: 300;
      padding-top: max(18px, env(safe-area-inset-top));
      padding-bottom: max(16px, env(safe-area-inset-bottom));
      box-shadow: var(--shadow-lg);
    }
    .primary-navigation button,
    .search-launcher,
    .file-menu button {
      min-height: 44px;
    }
    .sidebar-footer select {
      min-height: 44px;
    }
    .sidebar-footer {
      padding-top: 6px;
    }
  }
</style>
