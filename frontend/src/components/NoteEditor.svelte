<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import { notes, backlinks as backlinksApi } from '../lib/api.js';
  import Backlinks from './Backlinks.svelte';

  const dispatch = createEventDispatcher();

  export let path = '';

  let note = null;
  let mode = 'view';
  let content = '';
  let html = '';
  let saveStatus = '';
  let backlinks = [];
  let editWidthPercent = 50;
  let isResizing = false;
  let saveTimeout;
  let previewTimeout;

  $: loadNote(path);

  async function loadNote(notePath) {
    if (!notePath) return;
    try {
      note = await notes.get(notePath);
      content = note.content;
      html = note.html;
      mode = localStorage.getItem('noteMode') || 'view';
      await loadBacklinks();
    } catch (e) {
      if (e.message.includes('404')) {
        note = { notFound: true, path: notePath };
      } else {
        console.error('Failed to load note:', e);
      }
    }
  }

  async function loadBacklinks() {
    try {
      backlinks = await backlinksApi.get(path);
    } catch (e) {
      backlinks = [];
    }
  }

  function setNoteMode(newMode) {
    mode = newMode;
    localStorage.setItem('noteMode', newMode);
    if (newMode === 'split' || newMode === 'view') {
      updatePreview();
    }
  }

  function handleContentChange() {
    saveStatus = 'Saving...';
    clearTimeout(saveTimeout);
    saveTimeout = setTimeout(async () => {
      try {
        await notes.save(path, content);
        saveStatus = 'Saved';
        setTimeout(() => saveStatus = '', 2000);
      } catch (e) {
        saveStatus = 'Error';
      }
    }, 500);

    if (mode === 'split' || mode === 'view') {
      clearTimeout(previewTimeout);
      previewTimeout = setTimeout(updatePreview, 300);
    }
  }

  async function updatePreview() {
    try {
      html = await notes.preview(path, content);
    } catch (e) {
      console.error('Preview failed:', e);
    }
  }

  function startResize(e) {
    isResizing = true;
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';

    const container = document.getElementById('note-content-area');
    const containerRect = container.getBoundingClientRect();

    function onMouseMove(e) {
      if (!isResizing) return;
      const x = e.clientX - containerRect.left;
      const percent = (x / containerRect.width) * 100;
      editWidthPercent = Math.max(20, Math.min(80, percent));
    }

    function onMouseUp() {
      isResizing = false;
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
      localStorage.setItem('splitRatio', editWidthPercent);
      document.removeEventListener('mousemove', onMouseMove);
      document.removeEventListener('mouseup', onMouseUp);
    }

    document.addEventListener('mousemove', onMouseMove);
    document.addEventListener('mouseup', onMouseUp);
  }

  async function createNote() {
    try {
      await notes.create(path);
      await loadNote(path);
      setNoteMode('edit');
    } catch (e) {
      alert('Failed to create note: ' + e.message);
    }
  }

  function buildBreadcrumbs(notePath) {
    const parts = notePath.split('/');
    const crumbs = [];
    for (let i = 0; i < parts.length - 1; i++) {
      crumbs.push({
        name: parts[i],
        path: parts.slice(0, i + 1).join('/')
      });
    }
    return crumbs;
  }

  onMount(() => {
    const savedRatio = localStorage.getItem('splitRatio');
    if (savedRatio) editWidthPercent = parseFloat(savedRatio);
  });
</script>

{#if note?.notFound}
  <div class="not-found">
    <svg xmlns="http://www.w3.org/2000/svg" width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1" stroke-linecap="round" stroke-linejoin="round" class="opacity-40">
      <path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z"/>
      <polyline points="14 2 14 8 20 8"/>
    </svg>
    <h2>Note not found</h2>
    <p class="opacity-50">{path}</p>
    <button on:click={createNote} class="btn-primary">Create this note</button>
  </div>
{:else if note}
  <div class="note-editor" data-note-path={path} data-note-title={note.title}>
    <!-- Unified Header -->
    <div class="editor-header">
      <div class="header-left">
        <!-- Breadcrumbs -->
        <nav class="breadcrumbs">
          {#each buildBreadcrumbs(path) as crumb}
            <a href="/note/{crumb.path}" on:click|preventDefault={() => dispatch('navigate', crumb.path)}>{crumb.name}</a>
            <span class="sep">/</span>
          {/each}
          <span class="current">{note.title}</span>
        </nav>
        {#if saveStatus}
          <span class="save-status">{saveStatus}</span>
        {/if}
      </div>
      <div class="header-right">
        <button on:click={() => setNoteMode('view')} class="mode-btn" class:active={mode === 'view'} title="View mode">
          <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/></svg>
        </button>
        <button on:click={() => setNoteMode('split')} class="mode-btn" class:active={mode === 'split'} title="Split mode">
          <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="18" x="3" y="3" rx="2" ry="2"/><line x1="12" y1="3" x2="12" y2="21"/></svg>
        </button>
        <button on:click={() => setNoteMode('edit')} class="mode-btn" class:active={mode === 'edit'} title="Edit mode">
          <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 3a2.85 2.83 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5Z"/></svg>
        </button>
      </div>
    </div>

    <!-- Content Area -->
    <div class="content-area" id="note-content-area">
      {#if mode !== 'view'}
        <div class="edit-pane" style="width: {mode === 'split' ? editWidthPercent + '%' : '100%'};">
          <textarea bind:value={content} on:input={handleContentChange}></textarea>
        </div>
      {/if}

      {#if mode === 'split'}
        <div class="divider" on:mousedown={startResize} role="separator" aria-label="Resize panes">
          <div class="divider-handle">
            <div class="dot"></div>
            <div class="dot"></div>
            <div class="dot"></div>
          </div>
        </div>
      {/if}

      {#if mode !== 'edit'}
        <div class="preview-pane">
          <div class="markdown-content">
            {@html html}
            <Backlinks {backlinks} on:navigate />
          </div>
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .note-editor {
    display: flex;
    flex-direction: column;
    height: 100%;
  }

  .not-found {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 100%;
    gap: 1rem;
    padding: 2rem;
    text-align: center;
  }

  .not-found h2 {
    margin: 0;
    font-size: 1.25rem;
    font-weight: 600;
  }

  .editor-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 1rem;
    height: var(--header-h);
    border-bottom: 1px solid var(--border-color);
    flex-shrink: 0;
    background: var(--bg);
  }

  .header-left {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    min-width: 0;
  }

  .breadcrumbs {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    font-size: 0.8125rem;
    color: var(--color-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .breadcrumbs a {
    color: inherit;
    text-decoration: none;
    transition: color 0.15s;
  }

  .breadcrumbs a:hover {
    color: var(--primary);
  }

  .breadcrumbs .current {
    font-weight: 600;
    color: var(--color);
  }

  .breadcrumbs .sep {
    opacity: 0.4;
  }

  .save-status {
    font-size: 0.6875rem;
    color: var(--color-faint);
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .header-right {
    display: flex;
    align-items: center;
    gap: 0.125rem;
    flex-shrink: 0;
  }

  .mode-btn {
    padding: 0.4rem;
    border-radius: var(--radius-sm);
    background: none;
    border: none;
    color: var(--color-faint);
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: all 0.15s;
  }

  .mode-btn:hover {
    background: var(--hover-bg);
    color: var(--color-muted);
  }

  .mode-btn.active {
    background: var(--primary-soft);
    color: var(--primary);
  }

  .content-area {
    flex: 1;
    display: flex;
    overflow: hidden;
  }

  .edit-pane {
    overflow-y: auto;
  }

  .edit-pane textarea {
    width: 100%;
    height: 100%;
    resize: none;
    border: none;
    outline: none;
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    font-size: 0.875rem;
    line-height: 1.7;
    background: transparent;
    color: inherit;
    padding: 1.5rem;
    box-sizing: border-box;
    tab-size: 2;
  }

  .divider {
    width: 4px;
    cursor: col-resize;
    background: transparent;
    display: flex;
    align-items: center;
    justify-content: center;
    user-select: none;
    position: relative;
    flex-shrink: 0;
  }

  .divider-handle {
    position: absolute;
    width: 16px;
    height: 32px;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 3px;
    background: var(--border-color);
    border-radius: 3px;
    transition: background 0.15s;
  }

  .divider:hover .divider-handle {
    background: var(--primary);
  }

  .dot {
    width: 3px;
    height: 3px;
    background: var(--color-faint);
    border-radius: 50%;
  }

  .preview-pane {
    flex: 1;
    overflow-y: auto;
    border-left: 1px solid var(--border-color);
  }

  .preview-pane .markdown-content {
    padding: 2rem 3rem;
  }

  .btn-primary {
    padding: 0.5rem 1.25rem;
    border-radius: var(--radius-sm);
    font-size: 0.875rem;
    font-weight: 600;
    background: var(--primary);
    color: var(--primary-color);
    border: none;
    cursor: pointer;
    transition: background 0.15s;
  }

  .btn-primary:hover {
    background: var(--primary-hover);
  }

  /* Mobile */
  @media (max-width: 768px) {
    .preview-pane .markdown-content {
      padding: 1.25rem;
    }

    .edit-pane textarea {
      padding: 1.25rem;
    }

    .editor-header {
      padding: 0 0.75rem;
    }

    .breadcrumbs {
      font-size: 0.75rem;
    }
  }
</style>
