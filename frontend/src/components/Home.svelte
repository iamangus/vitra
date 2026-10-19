<script>
  import { createEventDispatcher } from "svelte";
  import Icon from "./Icon.svelte";
  import EmptyState from "./EmptyState.svelte";
  export let notes = [];
  export let recent = [];
  const dispatch = createEventDispatcher();
  $: available = new Set(notes.map((note) => note.path));
  $: opened = recent.filter((note) => available.has(note.path));
  $: displayed = opened.length ? opened : notes.slice(0, 8);
</script>

<div class="page">
  <div class="page-inner home">
    <div class="eyebrow">Your knowledge, connected</div>
    <h1>A little space to think clearly.</h1>
    <p class="page-description">
      Capture an idea. Follow a connection. Pick up where you left off.
    </p>
    <div class="home-actions">
      <button class="button primary" on:click={() => dispatch("create")}
        ><Icon name="plus" size={16} />New note</button
      ><button class="button" on:click={() => dispatch("search")}
        ><Icon name="search" size={16} />Find a note<kbd>⌘ / Ctrl K</kbd
        ></button
      >
    </div>
    {#if !notes.length}
      <div class="welcome">
        <EmptyState
          title="Make room for your first idea"
          message="Your vault starts with a Markdown note. Add a thought, a useful reference, or something you don’t want to forget."
          icon="edit"
          compact
          ><button class="button" on:click={() => dispatch("create")}
            >Create your first note<Icon name="arrow" size={15} /></button
          ></EmptyState
        >
      </div>
    {:else}
      <section
        aria-label={opened.length ? "Recently opened" : "From your vault"}
      >
        <div class="list-heading">
          <h2>{opened.length ? "Recently opened" : "From your vault"}</h2>
          <span
            >{notes.length} note{notes.length === 1 ? "" : "s"} in your vault</span
          >
        </div>
        <div class="note-list">
          {#each displayed as note}<a
              href={"/note/" + encodeURIComponent(note.path)}
              on:click|preventDefault={() => dispatch("navigate", note.path)}
              ><span class="note-icon"><Icon name="note" /></span><span
                class="note-text"
                ><strong>{note.title}</strong><small
                  >{note.path.includes("/")
                    ? note.path.slice(0, note.path.lastIndexOf("/"))
                    : "Vault root"}</small
                ></span
              ><Icon name="arrow" size={16} /></a
            >{/each}
        </div>
      </section>
    {/if}
    <div class="home-foot">
      <Icon name="link" size={15} /><span
        >Use <code>[[WikiLinks]]</code> to connect your notes. Your files stay Markdown.</span
      >
    </div>
  </div>
</div>

<style>
  .home {
    max-width: 800px;
    padding-top: 18px;
  }
  h1 {
    max-width: 560px;
    font-size: clamp(30px, 4vw, 42px) !important;
    margin-top: 16px !important;
  }
  .home-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    margin: 28px 0 56px;
  }
  kbd {
    color: var(--color-muted);
    font: 10px inherit;
    margin-left: 20px;
  }
  .list-heading {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 12px;
  }
  .list-heading h2 {
    font-size: 14px;
    font-weight: 550;
    margin: 0;
  }
  .list-heading span {
    color: var(--color-muted);
    font-size: 11px;
  }
  .note-list a {
    display: flex;
    gap: 14px;
    align-items: center;
    min-height: 76px;
    text-decoration: none;
    color: var(--color);
    border-bottom: 1px solid var(--border-color);
    padding: 14px 4px;
  }
  .note-list a:hover {
    background: var(--hover-bg);
    border-radius: 7px;
  }
  .note-icon {
    width: 36px;
    height: 40px;
    display: grid;
    place-items: center;
    border: 1px solid var(--border-color);
    border-radius: 7px;
    background: var(--bg-elevated);
    color: var(--color-muted);
  }
  .note-text {
    flex: 1;
    min-width: 0;
  }
  strong {
    display: block;
    font-size: 14px;
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  small {
    display: block;
    font-size: 11px;
    color: var(--color-muted);
    margin-top: 5px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .note-list a > :last-child {
    color: var(--color-muted);
  }
  .home-foot {
    display: flex;
    align-items: flex-start;
    gap: 9px;
    font-size: 12px;
    color: var(--color-muted);
    margin-top: 40px;
    line-height: 1.7;
  }
  .home-foot :global(svg) {
    margin-top: 3px;
  }
  code {
    font-size: 11px;
  }
  .welcome {
    border: 1px dashed var(--border-strong);
    border-radius: 12px;
  }
  @media (max-width: 768px) {
    .home {
      padding-top: 8px;
    }
    .home-actions {
      margin-bottom: 36px;
    }
    kbd {
      display: none;
    }
    .list-heading {
      flex-wrap: wrap;
    }
  }
</style>
