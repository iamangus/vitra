<script>
  import { createEventDispatcher } from "svelte";
  import Icon from "./Icon.svelte";
  export let backlinks = [];
  const dispatch = createEventDispatcher();
</script>

<section class="backlinks-section" aria-label="Linked from">
  <h2>
    <Icon name="link" size={15} />Linked from<span>{backlinks.length}</span>
  </h2>
  {#if backlinks.length}<div class="backlinks-list">
      {#each backlinks as link}<a
          href={"/note/" + encodeURIComponent(link.path)}
          on:click|preventDefault={() => dispatch("navigate", link.path)}
          ><Icon name="note" size={16} /><span
            ><strong>{link.title}</strong><small
              >{link.path.includes("/")
                ? link.path.slice(0, link.path.lastIndexOf("/"))
                : "Vault root"}</small
            ></span
          ><Icon name="arrow" size={14} /></a
        >{/each}
    </div>
  {:else}<p>
      No incoming links yet. Reference this note from another page to connect
      them.
    </p>{/if}
</section>

<style>
  .backlinks-section {
    margin-top: 52px;
    padding-top: 24px;
    border-top: 1px solid var(--border-color);
  }
  h2 {
    display: flex;
    gap: 8px;
    align-items: center;
    font-size: 12px;
    font-weight: 550;
    margin: 0 0 14px;
  }
  h2 > span {
    font-size: 10px;
    color: var(--color-muted);
    background: var(--hover-bg);
    border-radius: 4px;
    padding: 1px 5px;
  }
  .backlinks-list a {
    display: flex;
    align-items: center;
    gap: 11px;
    padding: 10px 4px;
    color: var(--color-muted);
    text-decoration: none;
    border-radius: 6px;
  }
  .backlinks-list a:hover {
    background: var(--hover-bg);
  }
  .backlinks-list a > span {
    min-width: 0;
    flex: 1;
  }
  strong {
    display: block;
    font-size: 12px;
    font-weight: 500;
    color: var(--color);
    overflow-wrap: anywhere;
  }
  small {
    display: block;
    margin-top: 3px;
    font-size: 10px;
  }
  p {
    font-size: 12px;
    color: var(--color-muted);
    line-height: 1.7;
    margin: 0;
  }
  @media (max-width: 768px) {
    .backlinks-list a {
      min-height: 52px;
    }
  }
</style>
