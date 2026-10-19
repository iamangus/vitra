<script>
  import { onMount, createEventDispatcher } from "svelte";
  import { activity } from "../lib/api.js";
  import { subscribeToLiveUpdates } from "../lib/live.js";
  import Icon from "./Icon.svelte";
  import EmptyState from "./EmptyState.svelte";
  const dispatch = createEventDispatcher();
  let entries = [];
  let loading = true;
  let error = "";
  let reloadTimer;
  let generation = 0;
  $: dates = [...new Set(entries.map((entry) => entry.date))];

  async function loadFeed() {
    const id = ++generation;
    try {
      const result = await activity.list({ limit: "100" });
      if (id === generation) {
        entries = result || [];
        error = "";
      }
    } catch {
      if (id === generation)
        error = "Activity could not load. Check your connection and try again.";
    } finally {
      if (id === generation) loading = false;
    }
  }
  function formatDate(date) {
    const parsed = new Date(date + "T12:00:00Z");
    return Number.isNaN(parsed.getTime())
      ? date
      : new Intl.DateTimeFormat("en", {
          month: "long",
          day: "numeric",
          year: "numeric",
          timeZone: "UTC",
        }).format(parsed);
  }
  onMount(() => {
    loadFeed();
    const unsubscribe = subscribeToLiveUpdates((event) => {
      if (event.activity) {
        clearTimeout(reloadTimer);
        reloadTimer = setTimeout(loadFeed, 250);
      }
    });
    return () => {
      generation++;
      clearTimeout(reloadTimer);
      unsubscribe();
    };
  });
</script>

<div class="page">
  <div class="page-inner activity-page">
    <div class="eyebrow">The story of your vault</div>
    <h1>Activity</h1>
    <p class="page-description">
      Changes, decisions, and discoveries. A timeline from your Markdown logs.
    </p>
    {#if loading}<div
        class="feed-skeleton"
        aria-label="Loading activity"
        aria-busy="true"
      >
        {#each [1, 2, 3] as item}<div
            class="skeleton"
            style="width:80%;height:18px"
          ></div>
          <div class="skeleton" style="width:55%;height:12px"></div>{/each}
      </div>
    {:else if error}<EmptyState
        icon="warning"
        title="Activity is unavailable"
        message={error}
        compact
        ><button class="button" on:click={loadFeed}>Try again</button
        ></EmptyState
      >
    {:else if !entries.length}<EmptyState
        icon="activity"
        title="Every note has a story"
        message="Record updates in log.md files using date headings and activity entries. They’ll appear here as your vault evolves."
        compact
        ><details class="log-example">
          <summary>See the log format</summary>
          <pre>## 2026-10-19

* **Update**: Documented a useful idea.</pre>
        </details></EmptyState
      >
    {:else}<div class="timeline">
        {#each dates as date}<section aria-label={formatDate(date)}>
            <h2><time datetime={date}>{formatDate(date)}</time></h2>
            <div class="date-entries">
              {#each entries.filter((entry) => entry.date === date) as entry}<div
                  class="entry"
                >
                  <span class="timeline-dot"></span>
                  <div class="entry-header">
                    <span class="tag-chip">{entry.kind || "Update"}</span
                    ><button
                      class="entry-source"
                      title={"Open " + entry.source}
                      on:click={() => dispatch("navigate", entry.source)}
                      ><Icon name="note" size={13} />{entry.source_path === "."
                        ? "Vault log"
                        : entry.source_path || "Vault log"}<Icon
                        name="arrow"
                        size={12}
                      /></button
                    >
                  </div>
                  <p>{entry.text}</p>
                </div>{/each}
            </div>
          </section>{/each}
      </div>{/if}
  </div>
</div>

<style>
  .activity-page {
    max-width: 780px;
  }
  .timeline {
    margin-top: 40px;
  }
  section {
    margin-bottom: 34px;
  }
  h2 {
    font-size: 12px;
    font-weight: 550;
    margin: 0 0 18px;
  }
  .date-entries {
    border-left: 1px solid var(--border-strong);
    margin-left: 5px;
  }
  .entry {
    position: relative;
    padding: 0 0 24px 26px;
  }
  .timeline-dot {
    position: absolute;
    left: -4px;
    top: 9px;
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--primary);
    box-shadow: 0 0 0 4px var(--bg);
  }
  .entry-header {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 10px;
  }
  .entry-source {
    display: flex;
    align-items: center;
    gap: 6px;
    border: 0;
    background: none;
    color: var(--color-muted);
    font-size: 11px;
    padding: 4px 0;
    overflow-wrap: anywhere;
    max-width: 100%;
  }
  .entry-source:hover {
    color: var(--primary);
  }
  .entry p {
    font-size: 14px;
    line-height: 1.8;
    margin: 9px 0 0;
    overflow-wrap: anywhere;
  }
  .feed-skeleton {
    display: grid;
    gap: 20px;
    padding: 30px 0;
  }
  .log-example {
    text-align: left;
    max-width: 100%;
    font-size: 12px;
    color: var(--color-muted);
  }
  .log-example summary {
    cursor: pointer;
  }
  pre {
    padding: 16px;
    background: var(--code-bg);
    border-radius: 8px;
    overflow-x: auto;
    max-width: 100%;
    font-size: 11px;
  }
  @media (max-width: 768px) {
    .entry-source {
      min-height: 44px;
    }
    .entry {
      padding-left: 21px;
    }
  }
</style>
