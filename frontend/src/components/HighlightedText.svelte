<script>
  export let text = "";
  export let query = "";
  $: escaped = query.trim().replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  $: parts = escaped
    ? String(text).split(new RegExp("(" + escaped + ")", "ig"))
    : [String(text)];
</script>

{#each parts as part, index}{#if index % 2}<mark>{part}</mark
    >{:else}{part}{/if}{/each}

<style>
  mark {
    background: var(--primary-soft);
    color: var(--active-color);
    padding: 0 1px;
    border-radius: 2px;
  }
</style>
