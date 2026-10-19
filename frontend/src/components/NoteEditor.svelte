<script>
  import { createEventDispatcher, onMount, tick } from "svelte";
  import { notes, backlinks as backlinksApi } from "../lib/api.js";
  import { subscribeToLiveUpdates } from "../lib/live.js";
  import { getDraft, discardDraft } from "../lib/drafts.js";
  import Backlinks from "./Backlinks.svelte";
  import Icon from "./Icon.svelte";
  import Dialog from "./Dialog.svelte";
  import EmptyState from "./EmptyState.svelte";

  export let path = "";
  export let initialMode = "";
  export let mobile = false;
  const dispatch = createEventDispatcher();
  let note = null;
  let mode = "view";
  let content = "";
  let html = "";
  let saveStatus = "";
  let saveError = false;
  let loadError = "";
  let createError = "";
  let backlinks = [];
  let editWidthPercent = 50;
  let splitSupported = false;
  let isDirty = false;
  let showDiscard = false;
  let showOutline = false;
  let copyStatus = "";
  let headings = [];
  let previewElement;
  let draft;
  let unsubscribeDraft;
  let loadController;
  let previewController;
  let loadGeneration = 0;
  let previewTimeout;
  let noteRefreshTimeout;
  let backlinksRefreshTimeout;
  let resizeCleanup;
  $: noteType = note?.frontmatter?.type || "Note";
  $: noteTags = Array.isArray(note?.frontmatter?.tags)
    ? note.frontmatter.tags
    : [];
  $: effectiveMode = mode === "split" && !splitSupported ? "edit" : mode;
  $: loadNote(path);

  async function loadNote(notePath, options = { preserveMode: false }) {
    if (!notePath || (options.preserveMode && draft?.dirty())) return;
    const generation = ++loadGeneration;
    loadController?.abort();
    loadController = new AbortController();
    if (draft?.state.path !== notePath) {
      draft?.flush();
      unsubscribeDraft?.();
      note = null;
    }
    loadError = "";
    createError = "";
    try {
      const loaded = await notes.get(notePath, loadController.signal);
      if (generation !== loadGeneration) return;
      note = loaded;
      draft = getDraft(notePath, loaded);
      unsubscribeDraft?.();
      unsubscribeDraft = draft.subscribe((state) => {
        content = state.content;
        saveStatus = state.status;
        isDirty = draft.dirty();
        saveError = Boolean(state.error);
      });
      html = loaded.html;
      if (!options.preserveMode) {
        try {
          mode = initialMode || localStorage.getItem("noteMode") || "view";
        } catch {
          mode = initialMode || "view";
        }
        if (!["view", "edit", "split"].includes(mode)) mode = "view";
      }
      dispatch("loaded", { path: notePath, title: loaded.title });
      if (isDirty) updatePreview();
      loadBacklinks();
    } catch (error) {
      if (generation !== loadGeneration || error.name === "AbortError") return;
      if (error.status === 404) {
        note = { notFound: true, path: notePath };
        backlinks = [];
      } else
        loadError =
          "This note could not load. Check your connection and try again.";
    }
  }

  async function loadBacklinks() {
    const notePath = path;
    try {
      const result = await backlinksApi.get(notePath);
      if (notePath === path) backlinks = result;
    } catch {
      if (notePath === path) backlinks = [];
    }
  }

  function setNoteMode(value) {
    mode = value;
    try {
      localStorage.setItem("noteMode", value);
    } catch {}
    if (value !== "edit") updatePreview();
    if (value !== "edit" && draft && !draft.dirty())
      loadNote(path, { preserveMode: true });
  }

  function handleContentChange() {
    draft.edit(content);
    clearTimeout(previewTimeout);
    if (effectiveMode !== "edit")
      previewTimeout = setTimeout(updatePreview, 250);
  }

  async function updatePreview() {
    const notePath = path,
      text = content;
    previewController?.abort();
    const controller = new AbortController();
    previewController = controller;
    try {
      const result = await notes.preview(notePath, text, controller.signal);
      if (path === notePath && content === text && !controller.signal.aborted)
        html = result;
    } catch (error) {
      if (error.name !== "AbortError")
        console.error("Preview unavailable:", error);
    }
  }

  function confirmDiscard() {
    showDiscard = false;
    discardDraft(path);
    unsubscribeDraft?.();
    draft = null;
    loadNote(path, { preserveMode: true });
  }

  function handlePreviewClick(event) {
    const anchor = event.target.closest("a");
    if (
      !anchor ||
      event.ctrlKey ||
      event.metaKey ||
      event.shiftKey ||
      event.altKey
    )
      return;
    const url = new URL(anchor.href, window.location.href);
    if (
      url.origin === window.location.origin &&
      url.pathname.startsWith("/note/")
    ) {
      event.preventDefault();
      dispatch("navigate", decodeURIComponent(url.pathname.slice(6)));
    }
  }

  async function createNote() {
    try {
      await notes.create(path);
      await loadNote(path);
      setNoteMode("edit");
    } catch (error) {
      createError = error.message;
    }
  }

  function measure(node) {
    const observer = new ResizeObserver(
      (entries) => (splitSupported = entries[0].contentRect.width >= 760),
    );
    observer.observe(node);
    return { destroy: () => observer.disconnect() };
  }

  function startResize(event) {
    const bounds = event.currentTarget.parentElement.getBoundingClientRect();
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";
    const move = (event) =>
      (editWidthPercent = Math.max(
        20,
        Math.min(80, ((event.clientX - bounds.left) / bounds.width) * 100),
      ));
    const stop = () => {
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
      document.removeEventListener("mousemove", move);
      document.removeEventListener("mouseup", stop);
      try {
        localStorage.setItem("splitRatio", String(editWidthPercent));
      } catch {}
    };
    document.addEventListener("mousemove", move);
    document.addEventListener("mouseup", stop);
    resizeCleanup = stop;
  }

  function decorate(node, data) {
    let active = true;
    async function update(value) {
      await tick();
      if (!active || !node.isConnected) return;
      const first = node.querySelector(":scope > h1");
      if (first)
        first.hidden =
          first.textContent.trim().toLowerCase() ===
          value.title?.trim().toLowerCase();
      headings = [...node.querySelectorAll("h1:not([hidden]),h2,h3")].map(
        (heading, index) => {
          heading.id = `note-heading-${index}`;
          return {
            id: heading.id,
            title: heading.textContent,
            level: heading.tagName,
          };
        },
      );
      for (const pre of node.querySelectorAll("pre")) {
        pre.tabIndex = 0;
        pre.setAttribute("aria-label", "Code block");
        if (pre.querySelector("button")) continue;
        const button = document.createElement("button");
        button.className = "code-copy";
        button.type = "button";
        button.textContent = "Copy code";
        button.addEventListener("click", async () => {
          try {
            await navigator.clipboard.writeText(
              pre.querySelector("code")?.textContent || "",
            );
            button.textContent = "Copied";
          } catch {
            button.textContent = "Copy unavailable";
          }
        });
        pre.appendChild(button);
      }
      for (const table of node.querySelectorAll("table")) {
        if (table.parentElement.classList.contains("table-scroll")) continue;
        const wrapper = document.createElement("div");
        wrapper.className = "table-scroll";
        wrapper.tabIndex = 0;
        wrapper.setAttribute("role", "region");
        wrapper.setAttribute("aria-label", "Scrollable table");
        table.before(wrapper);
        wrapper.appendChild(table);
      }
    }
    update(data);
    return {
      update,
      destroy() {
        active = false;
      },
    };
  }

  async function jumpTo(id) {
    if (mobile) showOutline = false;
    await tick();
    previewElement
      ?.querySelector("#" + id)
      ?.scrollIntoView({
        block: "start",
        behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches
          ? "auto"
          : "smooth",
      });
  }

  async function copyDraft() {
    try {
      await navigator.clipboard.writeText(content);
      copyStatus = "Draft copied";
    } catch {
      copyStatus = "Select the Markdown text to copy your draft.";
    }
  }

  onMount(() => {
    try {
      const ratio = Number(localStorage.getItem("splitRatio"));
      if (Number.isFinite(ratio) && ratio >= 20 && ratio <= 80)
        editWidthPercent = ratio;
    } catch {}
    const unsubscribe = subscribeToLiveUpdates((event) => {
      if (event.graph) {
        clearTimeout(backlinksRefreshTimeout);
        backlinksRefreshTimeout = setTimeout(loadBacklinks, 250);
      }
      if (!(event.notes || event.tree) || (isDirty && !note?.notFound)) return;
      if (
        event.paths?.length &&
        !event.paths.some(
          (changed) =>
            changed === path ||
            path.startsWith(changed + "/") ||
            changed.startsWith(path + "/"),
        )
      )
        return;
      clearTimeout(noteRefreshTimeout);
      noteRefreshTimeout = setTimeout(
        () => loadNote(path, { preserveMode: true }),
        250,
      );
    });
    return () => {
      draft?.flush();
      unsubscribeDraft?.();
      loadGeneration++;
      loadController?.abort();
      previewController?.abort();
      clearTimeout(previewTimeout);
      clearTimeout(noteRefreshTimeout);
      clearTimeout(backlinksRefreshTimeout);
      resizeCleanup?.();
      unsubscribe();
    };
  });
</script>

{#if loadError}
  <EmptyState
    icon="warning"
    title="Unable to open this note"
    message={loadError}
    ><button class="button" on:click={() => loadNote(path)}>Try again</button
    ></EmptyState
  >
{:else if note?.notFound}
  <EmptyState
    title="This page is waiting for an idea"
    message={"No note exists at “" +
      path +
      "” yet. Create it to start writing."}
    ><button class="button primary" on:click={createNote}
      ><Icon name="plus" size={16} />Create this note</button
    >{#if createError}<p class="inline-error" role="alert">
        {createError}
      </p>{/if}</EmptyState
  >
{:else if note}
  <div
    class="note-editor"
    data-note-path={path}
    data-note-title={note.title}
    use:measure
  >
    <div class="document-toolbar">
      <div class="segmented" role="group" aria-label="Editor mode">
        <button
          title="View mode"
          aria-pressed={effectiveMode === "view"}
          on:click={() => setNoteMode("view")}>Read</button
        >
        <button
          title="Edit mode"
          aria-pressed={effectiveMode === "edit"}
          on:click={() => setNoteMode("edit")}>Edit</button
        >
        {#if !mobile}<button
            title={splitSupported
              ? "Split mode"
              : "Widen the window for split view"}
            disabled={!splitSupported}
            aria-pressed={effectiveMode === "split"}
            on:click={() => setNoteMode("split")}>Split</button
          >{/if}
      </div>
      <span
        class="save-status"
        class:error={saveError}
        role="status"
        aria-live="polite"
        ><span class="status-dot" class:saved={!isDirty && !saveError}
        ></span>{saveStatus || "All changes saved"}</span
      >
      {#if effectiveMode !== "edit"}<button
          class="button quiet outline-toggle"
          aria-label="Outline"
          aria-expanded={showOutline}
          on:click={() => (showOutline = !showOutline)}
          ><Icon name="outline" size={16} /><span>Outline</span></button
        >{/if}
    </div>
    {#if saveError || saveStatus === "Recovered unsaved draft"}
      <div class="save-banner" role="alert">
        <Icon name="warning" />
        <div>
          <strong
            >{saveStatus === "Recovered unsaved draft"
              ? "Your unsaved draft was recovered."
              : draft?.state.error?.status === 409
                ? "A newer version was saved elsewhere."
                : "Your note could not be saved."}</strong
          >
          <p>
            Your draft is retained in this tab. Copy it or resolve the conflict
            before leaving.
          </p>
          {#if copyStatus}<p>{copyStatus}</p>{/if}
          <div class="recovery-actions">
            <button class="button" on:click={() => draft.flush()}
              >{saveError ? "Retry save" : "Save recovered draft"}</button
            ><button class="button" on:click={copyDraft}>Copy draft</button
            ><button class="button quiet" on:click={() => (showDiscard = true)}
              >Discard draft and reload</button
            >
          </div>
        </div>
      </div>
    {/if}
    <div class="content-area" id="note-content-area">
      {#if effectiveMode !== "view"}
        <div
          class="edit-pane"
          style={"width:" +
            (effectiveMode === "split" ? editWidthPercent + "%" : "100%")}
        >
          <div class="source-label">
            <Icon name="edit" size={13} /><span>Markdown source</span><span
              class="source-path"
              title={path + ".md"}>{path}.md</span
            >
          </div>
          <textarea
            aria-label="Note markdown"
            bind:value={content}
            on:input={handleContentChange}
            spellcheck="false"
          ></textarea>
        </div>
      {/if}
      {#if effectiveMode === "split"}<button
          class="divider"
          aria-label="Resize panes"
          role="slider"
          aria-valuemin="20"
          aria-valuemax="80"
          aria-valuenow={editWidthPercent}
          aria-orientation="horizontal"
          on:mousedown={startResize}
          on:keydown={(event) => {
            if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
              event.preventDefault();
              editWidthPercent = Math.max(
                20,
                Math.min(
                  80,
                  editWidthPercent + (event.key === "ArrowRight" ? 5 : -5),
                ),
              );
            }
          }}
        ></button>{/if}
      {#if effectiveMode !== "edit"}
        <div class="preview-pane">
          <div class="reading-layout">
            <article class="document-column">
              <header class="document-heading">
                <div class="eyebrow">{noteType}</div>
                <h1>{note.title}</h1>
                <div class="note-meta">
                  <span class="note-location"
                    ><Icon name="folder" size={13} />{path.includes("/")
                      ? path.slice(0, path.lastIndexOf("/"))
                      : "Vault root"}</span
                  >{#each noteTags as tag}<span class="tag-chip"># {tag}</span
                    >{/each}
                </div>
              </header>
              <div
                class="markdown-content"
                bind:this={previewElement}
                use:decorate={{ html, title: note.title }}
                role="presentation"
                on:click={handlePreviewClick}
              >
                {@html html}
              </div>
              <Backlinks {backlinks} on:navigate />
            </article>
            {#if showOutline && !mobile}<aside
                class="outline"
                aria-label="On this page"
              >
                <div class="outline-heading">
                  <span class="eyebrow">On this page</span><button
                    class="icon-button"
                    aria-label="Close outline"
                    on:click={() => (showOutline = false)}
                    ><Icon name="close" size={14} /></button
                  >
                </div>
                {#if headings.length}{#each headings as heading}<button
                      class:subheading={heading.level === "H3"}
                      on:click={() => jumpTo(heading.id)}
                      >{heading.title}</button
                    >{/each}{:else}<p>
                    Headings in this note will appear here.
                  </p>{/if}
              </aside>{/if}
          </div>
        </div>
      {/if}
    </div>
  </div>
{:else}
  <div class="note-skeleton" aria-label="Loading note" aria-busy="true">
    <div class="skeleton" style="width:100px;height:12px"></div>
    <div class="skeleton" style="width:65%;height:34px"></div>
    {#each [95, 80, 90, 60] as width}<div
        class="skeleton"
        style={"width:" + width + "%;height:14px"}
      ></div>{/each}
  </div>
{/if}

{#if showDiscard}<Dialog
    title="Discard your local draft?"
    description="Your unsaved changes will be removed from this tab. The latest saved version will be loaded instead. This cannot be undone."
    on:cancel={() => (showDiscard = false)}
    ><svelte:fragment slot="actions"
      ><button class="button" on:click={() => (showDiscard = false)}
        >Keep draft</button
      ><button class="button danger" on:click={confirmDiscard}
        >Discard draft</button
      ></svelte:fragment
    ></Dialog
  >{/if}
{#if showOutline && mobile}<Dialog
    title="On this page"
    on:cancel={() => (showOutline = false)}
    ><div class="mobile-outline">
      {#each headings as heading}<button
          class="button quiet"
          on:click={() => jumpTo(heading.id)}>{heading.title}</button
        >{:else}<p class="muted">
          Add headings to give this note an outline.
        </p>{/each}
    </div>
    <svelte:fragment slot="actions"
      ><button class="button" on:click={() => (showOutline = false)}
        >Close</button
      ></svelte:fragment
    ></Dialog
  >{/if}

<style>
  .note-editor {
    display: flex;
    flex-direction: column;
    min-height: 0;
    flex: 1;
  }
  .document-toolbar {
    display: flex;
    align-items: center;
    gap: 18px;
    padding: 15px 32px;
    border-bottom: 1px solid var(--border-color);
    min-height: 66px;
    flex-shrink: 0;
  }
  .save-status {
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--color-muted);
    font-size: 11px;
    min-width: 0;
  }
  .save-status.error {
    color: var(--danger);
  }
  .status-dot {
    width: 5px;
    height: 5px;
    border-radius: 50%;
    background: currentColor;
    flex-shrink: 0;
  }
  .status-dot.saved {
    background: var(--success);
  }
  .outline-toggle {
    margin-left: auto;
    min-width: 34px;
    padding: 6px;
  }
  .content-area {
    display: flex;
    min-height: 0;
    flex: 1;
    position: relative;
  }
  .preview-pane {
    flex: 1;
    min-width: 0;
    overflow-y: auto;
  }
  .reading-layout {
    display: flex;
    align-items: flex-start;
    justify-content: center;
    max-width: 1160px;
    margin: auto;
  }
  .document-column {
    width: 100%;
    max-width: 780px;
    padding: 44px clamp(24px, 5vw, 64px) 72px;
    min-width: 0;
  }
  .document-heading {
    margin-bottom: 34px;
  }
  .document-heading h1 {
    font-size: clamp(28px, 3vw, 36px);
    font-weight: 600;
    letter-spacing: -0.045em;
    line-height: 1.2;
    margin: 13px 0 16px;
    overflow-wrap: anywhere;
  }
  .note-meta {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    align-items: center;
    color: var(--color-muted);
    font-size: 11px;
  }
  .note-location {
    display: flex;
    align-items: center;
    gap: 5px;
    overflow-wrap: anywhere;
    margin-right: 5px;
  }
  .edit-pane {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .source-label {
    display: flex;
    align-items: center;
    gap: 7px;
    padding: 14px 24px 8px;
    font-size: 10px;
    color: var(--color-muted);
    flex-shrink: 0;
  }
  .source-path {
    margin-left: auto;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 50%;
  }
  textarea {
    flex: 1;
    width: 100%;
    min-height: 0;
    resize: none;
    border: 0;
    border-radius: 0;
    background: var(--bg);
    font: 13px/1.9 var(--font-mono);
    padding: 20px 28px 40px;
    tab-size: 2;
  }
  textarea:focus-visible {
    outline-offset: -3px;
  }
  .divider {
    width: 5px;
    padding: 0;
    border: 0;
    border-left: 1px solid var(--border-color);
    border-radius: 0;
    background: var(--bg);
    cursor: col-resize;
    flex-shrink: 0;
  }
  .divider:hover,
  .divider:focus-visible {
    background: var(--primary-soft);
  }
  .save-banner {
    display: flex;
    gap: 12px;
    padding: 18px 32px;
    background: var(--warning-bg);
    color: var(--warning);
    border-bottom: 1px solid var(--border-color);
    max-height: 40%;
    overflow-y: auto;
    flex-shrink: 0;
  }
  .save-banner strong {
    font-size: 13px;
    font-weight: 600;
  }
  .save-banner p {
    margin: 4px 0 12px;
    font-size: 12px;
  }
  .recovery-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .recovery-actions .button {
    font-size: 11px;
    min-height: 32px;
  }
  .outline {
    flex: 0 0 210px;
    position: sticky;
    top: 28px;
    border-left: 1px solid var(--border-color);
    padding: 0 20px;
    margin: 42px 16px 20px 0;
    max-height: 65dvh;
    overflow-y: auto;
  }
  .outline-heading {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 8px;
  }
  .outline > button {
    display: block;
    width: 100%;
    border: 0;
    background: none;
    text-align: left;
    padding: 7px 0;
    font-size: 11px;
    color: var(--color-muted);
  }
  .outline > button:hover {
    color: var(--primary);
  }
  .outline .subheading {
    padding-left: 10px;
  }
  .outline p {
    font-size: 12px;
    color: var(--color-muted);
  }
  .mobile-outline {
    display: flex;
    flex-direction: column;
    align-items: stretch;
  }
  .mobile-outline button {
    text-align: left;
    justify-content: flex-start;
  }
  .note-skeleton {
    max-width: 650px;
    width: 100%;
    padding: 65px 32px;
    margin: 0 auto;
    display: grid;
    gap: 24px;
  }
  :global(.markdown-content [hidden]) {
    display: none;
  }
  :global(.markdown-content pre) {
    position: relative;
    padding-top: 46px;
  }
  :global(.code-copy) {
    position: absolute;
    top: 8px;
    right: 8px;
    border: 1px solid var(--border-strong);
    color: var(--color-muted);
    background: var(--bg-elevated);
    padding: 4px 8px;
    font:
      10px "Inter Variable",
      sans-serif;
  }
  :global(.table-scroll) {
    max-width: 100%;
    overflow-x: auto;
    margin: 0 0 24px;
  }
  :global(.markdown-content .table-scroll table) {
    display: table;
    overflow: visible;
    max-width: none;
    margin: 0;
  }
  @media (max-width: 1150px) {
    .outline {
      position: absolute;
      top: 0;
      right: 0;
      background: var(--bg-elevated);
      border: 1px solid var(--border-strong);
      border-radius: 10px;
      box-shadow: var(--shadow-md);
      padding: 12px 16px;
      width: 220px;
      max-width: 80%;
      margin: 8px 16px;
      z-index: 10;
    }
  }
  @media (max-width: 768px) {
    .document-toolbar {
      gap: 10px;
      padding: 10px 16px;
      flex-wrap: wrap;
    }
    .save-status {
      font-size: 10px;
      margin-left: auto;
    }
    .outline-toggle {
      margin-left: 0;
      min-height: 40px;
    }
    .outline-toggle span {
      display: none;
    }
    .document-column {
      padding: 30px 22px 60px;
    }
    .save-banner {
      padding: 16px 20px;
      max-height: 45%;
    }
    textarea {
      font-size: 16px;
      padding: 16px 20px 40px;
    }
    .source-label {
      padding-left: 20px;
    }
    .recovery-actions .button {
      min-height: 44px;
      font-size: 12px;
    }
    :global(.code-copy) {
      min-height: 36px;
      top: 5px;
    }
  }
</style>
