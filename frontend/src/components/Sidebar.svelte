<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import { files, folders, fileOps } from '../lib/api.js';
  import { subscribeToLiveUpdates } from '../lib/live.js';
  import FileTree from './FileTree.svelte';
  import { theme } from '../stores/theme.js';

  const dispatch = createEventDispatcher();

  export let activePath = '';
  export let sidebarOpen = true;
  export let mobile = false;

  let treeData = [];
  let showNewFolderDialog = false;
  let newFolderName = '';
  let contextMenu = { show: false, x: 0, y: 0, path: '', isDir: false };
  let sidebarWidth = 280;
  let isResizing = false;
  let liveTreeReloadTimeout;

  async function loadTree() {
    try {
      treeData = await files.list(activePath);
    } catch (e) {
      console.error('Failed to load file tree:', e);
    }
  }

  function scheduleTreeReload() {
    clearTimeout(liveTreeReloadTimeout);
    liveTreeReloadTimeout = setTimeout(() => {
      loadTree();
    }, 250);
  }

  function startResize(e) {
    isResizing = true;
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
    window.addEventListener('mousemove', handleResize);
    window.addEventListener('mouseup', stopResize);
  }

  function handleResize(e) {
    if (!isResizing) return;
    const newWidth = e.clientX;
    if (newWidth >= 200 && newWidth <= 420) {
      sidebarWidth = newWidth;
    }
  }

  function stopResize() {
    isResizing = false;
    document.body.style.cursor = '';
    document.body.style.userSelect = '';
    window.removeEventListener('mousemove', handleResize);
    window.removeEventListener('mouseup', stopResize);
  }

  function handleNewNote() {
    const name = prompt('New note name:');
    if (name) dispatch('navigate', name);
  }

  async function handleCreateFolder() {
    if (!newFolderName) return;
    try {
      await folders.create(newFolderName);
      newFolderName = '';
      showNewFolderDialog = false;
      await loadTree();
    } catch (e) {
      alert('Failed to create folder: ' + e.message);
    }
  }

  function handleContextMenu(e) {
    e.preventDefault();
    const item = e.target.closest('[data-path]');
    if (!item) return;
    contextMenu = {
      show: true,
      x: e.clientX,
      y: e.clientY,
      path: item.dataset.path,
      isDir: item.dataset.isdir === 'true'
    };
  }

  function closeContextMenu() {
    contextMenu.show = false;
  }

  async function ctxRename() {
    if (!contextMenu.path) return;
    const newName = prompt('Rename to:', contextMenu.path);
    if (newName && newName !== contextMenu.path) {
      try {
        await fileOps.rename(contextMenu.path, newName);
        await loadTree();
      } catch (e) {
        alert('Failed to rename: ' + e.message);
      }
    }
    closeContextMenu();
  }

  async function ctxDelete() {
    if (!contextMenu.path) return;
    if (!confirm('Delete "' + contextMenu.path + '"?')) return;
    try {
      await fileOps.delete(contextMenu.path);
      await loadTree();
    } catch (e) {
      alert('Failed to delete: ' + e.message);
    }
    closeContextMenu();
  }

  async function ctxNewNote() {
    if (!contextMenu.path) return;
    const folder = contextMenu.isDir ? contextMenu.path : '';
    const name = prompt('New note name:');
    if (name) {
      const fullPath = folder ? folder + '/' + name : name;
      dispatch('navigate', fullPath);
    }
    closeContextMenu();
  }

  $: isCollapsed = !sidebarOpen;

  onMount(() => {
    loadTree();

    const unsubscribe = subscribeToLiveUpdates((event) => {
      if (event.tree) {
        scheduleTreeReload();
      }
    });

    return () => {
      clearTimeout(liveTreeReloadTimeout);
      unsubscribe();
      stopResize();
    };
  });
</script>

<!-- Floating toggle button when sidebar is collapsed (desktop only) -->
{#if isCollapsed && !mobile}
  <button on:click={() => dispatch('toggle')} class="sidebar-fab" title="Open sidebar">
    <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <line x1="3" y1="12" x2="21" y2="12"/><line x1="3" y1="6" x2="21" y2="6"/><line x1="3" y1="18" x2="21" y2="18"/>
    </svg>
  </button>
{/if}

<aside class="sidebar" class:collapsed={isCollapsed} class:mobile style="width: {mobile ? '280px' : (isCollapsed ? 0 : sidebarWidth) + 'px'}; min-width: {mobile ? '280px' : (isCollapsed ? 0 : sidebarWidth) + 'px'};">
  <!-- Header -->
  <div class="sidebar-header">
    <div class="header-left">
      <button on:click={() => dispatch('toggle')} class="icon-btn" title={isCollapsed ? 'Open sidebar' : 'Close sidebar'}>
        <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <line x1="3" y1="12" x2="21" y2="12"/><line x1="3" y1="6" x2="21" y2="6"/><line x1="3" y1="18" x2="21" y2="18"/>
        </svg>
      </button>
      <span class="logo">Vitra</span>
    </div>
    <div class="header-right">
      <button on:click={() => theme.toggle()} class="icon-btn" title="Toggle theme">
        <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="12" cy="12" r="5"/><path d="M12 1v2M12 21v2M4.22 4.22l1.42 1.42M18.36 18.36l1.42 1.42M1 12h2M21 12h2M4.22 19.78l1.42-1.42M18.36 5.64l1.42-1.42"/>
        </svg>
      </button>
      <button on:click={() => dispatch('graph')} class="icon-btn" title="Graph view">
        <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="18" cy="5" r="3"/><circle cx="6" cy="12" r="3"/><circle cx="18" cy="19" r="3"/>
          <line x1="8.59" y1="13.51" x2="15.42" y2="17.49"/><line x1="15.41" y1="6.51" x2="8.59" y2="10.49"/>
        </svg>
      </button>
      <button on:click={() => dispatch('search')} class="icon-btn" title="Search">
        <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>
        </svg>
      </button>
    </div>
  </div>

  <!-- Actions -->
  <div class="sidebar-actions">
    <button on:click={handleNewNote} class="btn-primary">
      <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z"/><polyline points="14 2 14 8 20 8"/><line x1="12" y1="18" x2="12" y2="12"/><line x1="9" y1="15" x2="15" y2="15"/>
      </svg>
      New Note
    </button>
    <button on:click={() => showNewFolderDialog = true} class="btn-secondary" title="New Folder">
      <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13c0 1.1.9 2 2 2Z"/><line x1="12" y1="10" x2="12" y2="16"/><line x1="9" y1="13" x2="15" y2="13"/>
      </svg>
    </button>
  </div>

  <!-- File Tree -->
  <div class="file-tree" on:contextmenu={handleContextMenu}>
    <FileTree nodes={treeData} {activePath} on:navigate />
  </div>

  <!-- Resize Handle (desktop only) -->
  {#if !mobile}
    <div class="resize-handle" on:mousedown={startResize}></div>
  {/if}
</aside>

<!-- Context Menu -->
{#if contextMenu.show}
  <div id="context-menu" style="left: {contextMenu.x}px; top: {contextMenu.y}px;" on:click|stopPropagation>
    <button on:click={ctxNewNote}>New Note</button>
    <button on:click={ctxRename}>Rename</button>
    <button on:click={ctxDelete} class="danger">Delete</button>
  </div>
  <div class="backdrop" on:click={closeContextMenu}></div>
{/if}

<!-- New Folder Dialog -->
{#if showNewFolderDialog}
  <div class="dialog-overlay" on:click|self={() => showNewFolderDialog = false}>
    <div class="dialog">
      <h3 class="dialog-title">New Folder</h3>
      <input type="text" bind:value={newFolderName} placeholder="folder/name" class="dialog-input" on:keydown={e => e.key === 'Enter' && handleCreateFolder()}>
      <div class="dialog-actions">
        <button on:click={() => showNewFolderDialog = false} class="btn-text">Cancel</button>
        <button on:click={handleCreateFolder} class="btn-primary">Create</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .sidebar {
    display: flex;
    flex-direction: column;
    background: var(--sidebar-bg);
    border-right: 1px solid var(--border-color);
    transition: width 0.25s ease, min-width 0.25s ease;
    overflow: hidden;
    position: relative;
    z-index: 100;
    height: 100vh;
  }

  .sidebar.collapsed {
    width: 0 !important;
    min-width: 0 !important;
    border-right: none;
  }

  .sidebar.mobile {
    position: fixed;
    left: 0;
    top: 0;
    bottom: 0;
    z-index: 100;
    box-shadow: var(--shadow-lg);
    transform: translateX(-100%);
    transition: transform 0.25s ease;
  }

  .sidebar.mobile:not(.collapsed) {
    transform: translateX(0);
  }

  .sidebar-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 0.75rem;
    height: var(--header-h);
    border-bottom: 1px solid var(--border-color);
    flex-shrink: 0;
  }

  .header-left {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .header-right {
    display: flex;
    align-items: center;
    gap: 0.25rem;
  }

  .logo {
    font-weight: 700;
    font-size: 1rem;
    letter-spacing: -0.02em;
    color: var(--color);
  }

  .icon-btn {
    padding: 0.4rem;
    border-radius: var(--radius-sm);
    background: none;
    border: none;
    color: var(--color-muted);
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: all 0.15s;
  }

  .icon-btn:hover {
    background: var(--hover-bg);
    color: var(--color);
  }

  .sidebar-actions {
    display: flex;
    gap: 0.5rem;
    padding: 0.75rem;
    border-bottom: 1px solid var(--border-color);
    flex-shrink: 0;
  }

  .btn-primary {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.375rem;
    padding: 0.5rem 0.75rem;
    border-radius: var(--radius-sm);
    font-size: 0.8125rem;
    font-weight: 600;
    background: var(--primary);
    color: var(--primary-color);
    border: none;
    cursor: pointer;
    flex: 1;
    transition: background 0.15s;
  }

  .btn-primary:hover {
    background: var(--primary-hover);
  }

  .btn-secondary {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0.5rem;
    border-radius: var(--radius-sm);
    font-size: 0.8125rem;
    font-weight: 500;
    background: var(--button-bg);
    color: var(--button-color);
    border: none;
    cursor: pointer;
    transition: background 0.15s;
  }

  .btn-secondary:hover {
    background: var(--hover-bg);
  }

  .btn-text {
    padding: 0.5rem 0.75rem;
    border-radius: var(--radius-sm);
    font-size: 0.875rem;
    background: none;
    border: none;
    color: var(--color-muted);
    cursor: pointer;
    transition: all 0.15s;
  }

  .btn-text:hover {
    background: var(--hover-bg);
    color: var(--color);
  }

  .file-tree {
    flex: 1;
    overflow-y: auto;
    padding: 0.5rem;
  }

  .resize-handle {
    position: absolute;
    right: -3px;
    top: 0;
    bottom: 0;
    width: 6px;
    cursor: col-resize;
    background: transparent;
    z-index: 10;
    transition: background 0.15s;
  }

  .resize-handle:hover {
    background: var(--primary);
  }

  .sidebar-fab {
    position: fixed;
    top: 0.625rem;
    left: 0.625rem;
    z-index: 95;
    padding: 0.5rem;
    border-radius: var(--radius-sm);
    background: var(--bg-elevated);
    border: 1px solid var(--border-color);
    color: var(--color-muted);
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    box-shadow: var(--shadow-md);
    transition: all 0.15s;
  }

  .sidebar-fab:hover {
    background: var(--hover-bg);
    color: var(--color);
  }

  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 50;
  }

  .dialog-overlay {
    position: fixed;
    inset: 0;
    background: var(--overlay-bg);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 200;
    backdrop-filter: blur(2px);
  }

  .dialog {
    background: var(--bg-elevated);
    color: var(--color);
    border-radius: var(--radius);
    padding: 1.5rem;
    width: 22rem;
    max-width: 90vw;
    box-shadow: var(--shadow-lg);
    border: 1px solid var(--border-color);
  }

  .dialog-title {
    font-weight: 600;
    margin: 0 0 1rem;
    font-size: 1rem;
  }

  .dialog-input {
    width: 100%;
    padding: 0.5rem 0.75rem;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border-color);
    font-size: 0.875rem;
    margin-bottom: 1rem;
    background: var(--bg);
    color: var(--color);
    outline: none;
    transition: border-color 0.15s;
  }

  .dialog-input:focus {
    border-color: var(--primary);
  }

  .dialog-actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
  }
</style>
