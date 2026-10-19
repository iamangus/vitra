<script>
  import { onMount, tick } from "svelte";
  import Sidebar from "./components/Sidebar.svelte";
  import NoteEditor from "./components/NoteEditor.svelte";
  import Search from "./components/Search.svelte";
  import Activity from "./components/Activity.svelte";
  import Home from "./components/Home.svelte";
  import Icon from "./components/Icon.svelte";
  import { hasUnsavedDrafts } from "./lib/drafts.js";
  import { theme } from "./stores/theme.js";

  let currentView = "home";
  let currentPath = "";
  let currentTitle = "";
  let searchQuery = "";
  let sidebarOpen = window.innerWidth > 768;
  let mobile = window.innerWidth <= 768;
  let sidebar;
  let GraphView;
  let graphError = "";
  let initialMode = "";
  let vaultNotes = [];
  let recent = [];
  try {
    recent = JSON.parse(localStorage.getItem("vitra:recent") || "[]")
      .filter(
        (item) =>
          typeof item?.path === "string" && typeof item?.title === "string",
      )
      .slice(0, 8);
  } catch {}

  $: breadcrumbs = currentPath
    .split("/")
    .slice(0, -1)
    .map((name, index, parts) => ({
      name,
      path: parts.slice(0, index + 1).join("/"),
    }));
  $: viewTitle =
    { home: "Notes", search: "Search", graph: "Graph", activity: "Activity" }[
      currentView
    ] ||
    currentTitle ||
    currentPath.split("/").pop();

  async function ensureGraph() {
    graphError = "";
    try {
      GraphView = (await import("./components/GraphView.svelte")).default;
    } catch {
      graphError =
        "The graph could not load. Check your connection and try again.";
    }
  }

  function navigate(view, path = "", options = {}) {
    currentView = view;
    currentPath = path;
    currentTitle = path.split("/").pop() || "";
    initialMode = options.edit ? "edit" : "";
    if (view === "graph") ensureGraph();
    const url =
      view === "home"
        ? "/"
        : view === "note"
          ? `/note/${encodeURIComponent(path)}`
          : `/${view}${view === "search" && searchQuery ? "?q=" + encodeURIComponent(searchQuery) : ""}`;
    window.history.pushState({}, "", url);
    if (mobile) sidebarOpen = false;
    if (view === "search") tick().then(() => document.getElementById("global-search-input")?.focus());
  }

  function handlePopState() {
    const pathname = window.location.pathname;
    initialMode = "";
    if (pathname.startsWith("/note/")) {
      try {
        currentPath = decodeURIComponent(pathname.slice(6));
      } catch {
        currentPath = "";
      }
      currentTitle = currentPath.split("/").pop();
      currentView = currentPath ? "note" : "home";
    } else {
      currentPath = "";
      currentView = ["search", "graph", "activity"].includes(pathname.slice(1))
        ? pathname.slice(1)
        : "home";
      if (currentView === "search")
        searchQuery =
          new URLSearchParams(window.location.search).get("q") || "";
      if (currentView === "graph") ensureGraph();
    }
    if (mobile) sidebarOpen = false;
  }

  async function revealFolder(path) {
    sidebarOpen = true;
    await tick();
    sidebar?.revealFolder(path);
  }

  function remember(note) {
    if (note.path !== currentPath) return;
    currentTitle = note.title;
    recent = [
      { path: note.path, title: note.title },
      ...recent.filter((item) => item.path !== note.path),
    ].slice(0, 8);
    try {
      localStorage.setItem("vitra:recent", JSON.stringify(recent));
    } catch {}
  }

  handlePopState();
  onMount(() => {
    const warn = (event) => {
      if (hasUnsavedDrafts()) {
        event.preventDefault();
        event.returnValue = "";
      }
    };
    const resize = () => {
      const next = window.innerWidth <= 768;
      if (next !== mobile) sidebarOpen = !next;
      mobile = next;
    };
    const keydown = (event) => {
      if (event.target instanceof Element && event.target.closest("dialog"))
        return;
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        navigate("search");
      }
      if (event.key === "Escape" && mobile) sidebarOpen = false;
    };
    window.addEventListener("beforeunload", warn);
    window.addEventListener("resize", resize);
    window.addEventListener("popstate", handlePopState);
    window.addEventListener("keydown", keydown);
    return () => {
      window.removeEventListener("beforeunload", warn);
      window.removeEventListener("resize", resize);
      window.removeEventListener("popstate", handlePopState);
      window.removeEventListener("keydown", keydown);
    };
  });
</script>

<a class="skip-link" href="#main-content">Skip to content</a>
<div class="app" data-theme={$theme}>
  {#if mobile && sidebarOpen}<button
      class="sidebar-backdrop"
      aria-label="Close navigation"
      on:click={() => (sidebarOpen = false)}
    ></button>{/if}
  <Sidebar
    bind:this={sidebar}
    activePath={currentPath}
    view={currentView}
    {sidebarOpen}
    {mobile}
    on:navigate={(event) => navigate("note", event.detail)}
    on:view={(event) => navigate(event.detail)}
    on:created={(event) => navigate("note", event.detail, { edit: true })}
    on:toggle={() => (sidebarOpen = !sidebarOpen)}
    on:tree={(event) => (vaultNotes = event.detail)}
  />
  <div class="workspace" inert={mobile && sidebarOpen}>
    <header class="workspace-bar">
      <button
        class="icon-button"
        aria-label={sidebarOpen ? "Close navigation" : "Open navigation"}
        aria-expanded={sidebarOpen}
        aria-controls="vault-navigation"
        on:click={() => (sidebarOpen = !sidebarOpen)}
        ><Icon name="menu" /></button
      >
      <nav class="breadcrumbs" aria-label="Breadcrumb">
        {#if currentView === "note"}
          {#each breadcrumbs as crumb, index}<span
              class="folder-crumb"
              class:earlier={index < breadcrumbs.length - 1}
              ><button
                on:click={() => revealFolder(crumb.path)}
                title={"Show folder: " + crumb.path}>{crumb.name}</button
              ><Icon name="chevron" size={12} /></span
            >{/each}
        {/if}
        <span class="current" aria-current="page" title={viewTitle}
          >{viewTitle}</span
        >
      </nav>
      {#if currentView !== "search"}<button
          class="icon-button quick-search"
          aria-label="Search notes"
          title="Search notes (Ctrl / ⌘ K)"
          on:click={() => navigate("search")}><Icon name="search" /></button
        >{/if}
    </header>
    <main id="main-content" tabindex="-1">
      {#if currentView === "home"}
        <Home
          notes={vaultNotes}
          {recent}
          on:navigate={(event) => navigate("note", event.detail)}
          on:create={() => sidebar?.openCreate("note")}
          on:search={() => navigate("search")}
        />
      {:else if currentView === "note"}
        <NoteEditor
          path={currentPath}
          {initialMode}
          {mobile}
          on:loaded={(event) => remember(event.detail)}
          on:navigate={(event) => navigate("note", event.detail)}
          on:folder={(event) => revealFolder(event.detail)}
        />
      {:else if currentView === "search"}
        <Search
          query={searchQuery}
          on:query={(event) => (searchQuery = event.detail)}
          on:navigate={(event) => navigate("note", event.detail)}
        />
      {:else if currentView === "activity"}
        <Activity on:navigate={(event) => navigate("note", event.detail)} />
      {:else if currentView === "graph"}
        {#if GraphView}<svelte:component
            this={GraphView}
            on:navigate={(event) => navigate("note", event.detail)}
          />
        {:else}<div class="graph-placeholder">
            <Icon name="graph" size={30} />
            <p>{graphError || "Opening your knowledge graph…"}</p>
            {#if graphError}<button class="button" on:click={ensureGraph}
                >Try again</button
              >{/if}
          </div>{/if}
      {/if}
    </main>
  </div>
</div>

<style>
  .app {
    display: flex;
    height: 100dvh;
    overflow: hidden;
    background: var(--bg);
  }
  .workspace {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  .workspace-bar {
    display: flex;
    align-items: center;
    gap: 12px;
    min-height: var(--header-h);
    padding: 0 24px;
    border-bottom: 1px solid var(--border-color);
    flex-shrink: 0;
  }
  .breadcrumbs {
    display: flex;
    gap: 10px;
    align-items: center;
    font-size: 12px;
    min-width: 0;
    flex: 1;
  }
  .folder-crumb {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
    color: var(--color-muted);
  }
  .folder-crumb button {
    border: 0;
    background: none;
    color: var(--color-muted);
    padding: 5px 0;
    white-space: nowrap;
    text-overflow: ellipsis;
    overflow: hidden;
    max-width: 160px;
  }
  .folder-crumb button:hover {
    color: var(--primary);
  }
  .current {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    font-weight: 500;
  }
  main {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    min-height: 0;
    outline: 0;
  }
  .sidebar-backdrop {
    position: fixed;
    inset: 0;
    background: var(--overlay-bg);
    border: 0;
    padding: 0;
    z-index: 250;
  }
  .skip-link {
    position: fixed;
    top: -100px;
    left: 12px;
    padding: 12px;
    background: var(--bg-elevated);
    z-index: 1000;
    border: 1px solid var(--primary);
  }
  .skip-link:focus {
    top: 12px;
  }
  .graph-placeholder {
    margin: auto;
    text-align: center;
    padding: 24px;
    color: var(--color-muted);
  }
  @media (max-width: 768px) {
    .workspace-bar {
      gap: 6px;
      padding: env(safe-area-inset-top) 12px 0;
      min-height: 60px;
    }
    .folder-crumb.earlier {
      display: none;
    }
    .folder-crumb button {
      max-width: 110px;
      min-height: 44px;
    }
  }
</style>
