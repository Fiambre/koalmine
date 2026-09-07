<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { GetTasks, RefreshNow, OpenURL } from '../../wailsjs/go/main/App.js'
  import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
  import type { providers } from '../../wailsjs/go/models'

  type Filter = 'all' | 'issue' | 'pr' | 'mention'

  const typeLabels: Record<string, string> = {
    issue: 'Issue',
    pr: 'PR',
    mention: 'Mención',
  }

  let tasks: providers.TaskItem[] = []
  let filter: Filter = 'all'
  let refreshing = false
  let loadedOnce = false
  let loadError = ''

  function onUpdated(items: providers.TaskItem[]) {
    tasks = items ?? []
    loadedOnce = true
    refreshing = false
  }

  onMount(async () => {
    EventsOn('tasks:updated', onUpdated)
    try {
      tasks = (await GetTasks()) ?? []
    } catch (e) {
      loadError = String(e)
    } finally {
      loadedOnce = true
    }
  })

  onDestroy(() => {
    EventsOff('tasks:updated')
  })

  async function refresh() {
    refreshing = true
    loadError = ''
    try {
      await RefreshNow()
    } catch (e) {
      loadError = String(e)
      refreshing = false
    }
  }

  function open(url: string) {
    OpenURL(url)
  }

  $: filtered = filter === 'all' ? tasks : tasks.filter((t) => t.type === filter)
</script>

<section class="tasks">
  <header>
    <div class="filters">
      {#each [['all', 'Todas'], ['issue', 'Issues'], ['pr', 'PRs'], ['mention', 'Menciones']] as [value, label] (value)}
        <button class:active={filter === value} on:click={() => (filter = value as Filter)}>{label}</button>
      {/each}
    </div>
    <button class="refresh" on:click={refresh} disabled={refreshing}>
      {refreshing ? 'Actualizando…' : 'Actualizar'}
    </button>
  </header>

  {#if loadError}
    <p class="status error">No se pudieron cargar las tareas: {loadError}</p>
  {/if}

  {#if !loadedOnce}
    <p class="hint">Cargando…</p>
  {:else if filtered.length === 0}
    <div class="empty-state">
      <p>{tasks.length === 0 ? 'Todavía no hay tareas para mostrar.' : 'Nada en este filtro.'}</p>
      {#if tasks.length === 0}
        <p class="hint">Si ya configuraste un proveedor, puede que aún no haya corrido el primer chequeo.</p>
      {/if}
    </div>
  {:else}
    <ul>
      {#each filtered as item (item.id)}
        <li>
          <button class="task" on:click={() => open(item.url)}>
            <span class="badge {item.type}">{typeLabels[item.type] ?? item.type}</span>
            <span class="title">{item.title}</span>
            <span class="meta">{item.project} · {item.provider}</span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  .tasks {
    text-align: left;
    max-width: 640px;
    margin: 0 auto;
    padding: 1.5rem;
  }

  header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 1rem;
    gap: 1rem;
  }

  .status.error {
    color: #ff8a8a;
    font-size: 0.85rem;
    margin: 0 0 0.75rem;
  }

  .filters {
    display: flex;
    gap: 0.4rem;
  }

  .filters button {
    border: none;
    background: rgba(255, 255, 255, 0.08);
    color: rgba(255, 255, 255, 0.75);
    padding: 0.3rem 0.7rem;
    border-radius: 4px;
    cursor: pointer;
    font-size: 0.85rem;
  }

  .filters button.active {
    background: rgba(255, 255, 255, 0.2);
    color: white;
  }

  .refresh {
    border: none;
    border-radius: 4px;
    padding: 0.4rem 0.9rem;
    cursor: pointer;
    background: #3d7bfd;
    color: white;
  }

  .refresh:disabled {
    opacity: 0.6;
    cursor: default;
  }

  .hint {
    opacity: 0.7;
  }

  .empty-state {
    text-align: center;
    padding: 2rem 0;
  }

  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .task {
    width: 100%;
    display: flex;
    align-items: center;
    gap: 0.6rem;
    text-align: left;
    background: rgba(255, 255, 255, 0.06);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 6px;
    padding: 0.6rem 0.8rem;
    cursor: pointer;
    color: white;
  }

  .task:hover {
    background: rgba(255, 255, 255, 0.12);
  }

  .badge {
    font-size: 0.7rem;
    padding: 0.15rem 0.4rem;
    border-radius: 3px;
    background: rgba(255, 255, 255, 0.15);
    flex-shrink: 0;
  }

  .badge.pr {
    background: #7c5cff;
  }

  .badge.mention {
    background: #ff9f43;
  }

  .title {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .meta {
    font-size: 0.75rem;
    opacity: 0.6;
    flex-shrink: 0;
  }
</style>
