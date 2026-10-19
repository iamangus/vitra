<script>
  import Sidebar from './components/Sidebar.svelte';
  import NoteEditor from './components/NoteEditor.svelte';
  import Search from './components/Search.svelte';
  import { onMount } from 'svelte';
  import { hasUnsavedDrafts } from './lib/drafts.js';
  import { theme } from './stores/theme.js';

  let currentView = 'home';
  let currentPath = '';
  let searchQuery = '';
  let sidebarOpen = true;
  let mobile = false;
  let GraphView;
  let graphError = '';

  async function ensureGraph() {
    try { GraphView = (await import('./components/GraphView.svelte')).default; }
    catch (error) { graphError = error.message; }
  }

  function checkMobile() {
    const next = window.innerWidth <= 768;
    if (next !== mobile) sidebarOpen = !next;
    mobile = next;
  }

  checkMobile();

  function navigateToNote(path) {
    path = path.replace(/\.md$/, '');
    currentPath = path;
    currentView = 'note';
    window.history.pushState({}, '', `/note/${encodeURIComponent(path)}`);
    if (mobile) sidebarOpen = false;
  }

  function navigateToSearch(query = '') {
    searchQuery = query;
    currentView = 'search';
    window.history.pushState({}, '', `/search${query ? '?q=' + encodeURIComponent(query) : ''}`);
    if (mobile) sidebarOpen = false;
  }

  function navigateHome() {
    currentView = 'home';
    currentPath = '';
    window.history.pushState({}, '', '/');
    if (mobile) sidebarOpen = false;
  }

  function navigateToGraph() {
    ensureGraph();
    currentPath = '';
    currentView = 'graph';
    window.history.pushState({}, '', '/graph');
    if (mobile) sidebarOpen = false;
  }

  function handlePopState() {
    const path = window.location.pathname;
    if (path.startsWith('/note/')) {
      currentPath = decodeURIComponent(path.slice(6));
      currentView = 'note';
    } else if (path === '/search') {
      const params = new URLSearchParams(window.location.search);
      searchQuery = params.get('q') || '';
      currentView = 'search';
    } else if (path === '/graph') {
      ensureGraph();
      currentPath = '';
      currentView = 'graph';
    } else if (path === '/') {
      currentView = 'home';
      currentPath = '';
    }
  }

  handlePopState();
  onMount(() => {
    const warn = event => { if (hasUnsavedDrafts()) { event.preventDefault(); event.returnValue = ''; } };
    window.addEventListener('beforeunload', warn);
    window.addEventListener('resize', checkMobile);
    window.addEventListener('popstate', handlePopState);
    return () => {
      window.removeEventListener('beforeunload', warn);
      window.removeEventListener('resize', checkMobile);
      window.removeEventListener('popstate', handlePopState);
    };
  });
</script>

<div class="app" data-theme={$theme} class:sidebar-open={sidebarOpen} class:mobile>
  <!-- Mobile overlay backdrop -->
  {#if mobile && sidebarOpen}
    <button class="sidebar-backdrop" aria-label="Close navigation" on:click={() => sidebarOpen = false}></button>
  {/if}
  {#if mobile && !sidebarOpen}
    <button class="mobile-nav-toggle" aria-label="Open navigation" aria-expanded={sidebarOpen} on:click={() => sidebarOpen = true}><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M3 6h18M3 12h18M3 18h18"/></svg></button>
  {/if}

  <Sidebar
    on:home={navigateHome}
    on:navigate={e => navigateToNote(e.detail)}
    on:search={() => navigateToSearch()}
    on:graph={() => navigateToGraph()}
    on:toggle={() => sidebarOpen = !sidebarOpen}
    activePath={currentPath}
    {sidebarOpen}
    {mobile}
  />

  <main class="main">
    {#if currentView === 'home'}
      <div class="empty-state">
        <svg xmlns="http://www.w3.org/2000/svg" width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1" stroke-linecap="round" stroke-linejoin="round" class="empty-icon">
          <path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z"/>
          <polyline points="14 2 14 8 20 8"/>
        </svg>
        <p class="empty-title">Select a note</p>
        <p class="empty-hint">Choose a note from the sidebar to start reading or editing.</p>
      </div>
    {:else if currentView === 'note'}
      <NoteEditor path={currentPath} on:navigate={e => navigateToNote(e.detail)} />
    {:else if currentView === 'search'}
      <Search query={searchQuery} on:navigate={e => navigateToNote(e.detail)} />
    {:else if currentView === 'graph'}
      {#if GraphView}<svelte:component this={GraphView} on:navigate={e => navigateToNote(e.detail)} />
      {:else}<p role="status">{graphError || 'Loading graph…'}</p>{/if}
    {/if}
  </main>
</div>

<style>
  .app {
    display: flex;
    height: 100vh;
    overflow: hidden;
    background: var(--bg);
    color: var(--color);
  }

  .main {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    background: var(--bg);
    position: relative;
  }

  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 100%;
    gap: 0.75rem;
    padding: 2rem;
    text-align: center;
  }

  .empty-icon {
    color: var(--color-faint);
    margin-bottom: 0.5rem;
  }

  .empty-title {
    font-size: 1.125rem;
    font-weight: 600;
    margin: 0;
    color: var(--color-muted);
  }

  .empty-hint {
    font-size: 0.875rem;
    margin: 0;
    color: var(--color-faint);
    max-width: 280px;
  }

  .sidebar-backdrop {
    border: 0;
    padding: 0;
    position: fixed;
    inset: 0;
    background: var(--overlay-bg);
    z-index: 90;
    backdrop-filter: blur(2px);
  }

  .mobile-nav-toggle { position: fixed; bottom: 1rem; left: 1rem; z-index: 110; min-width: 44px; min-height: 44px; border: 1px solid var(--border-color); border-radius: var(--radius); background: var(--bg-elevated); color: var(--primary); cursor: pointer; }

  /* Mobile */
  @media (max-width: 768px) {
    .app.mobile .main {
      width: 100%;
    }
  }
</style>
