<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { _ } from 'svelte-i18n'
  import Settings from './lib/Settings.svelte'
  import TaskList from './lib/TaskList.svelte'
  import PanelForm from './lib/PanelForm.svelte'
  import { GetTasks, ListPanels } from '../wailsjs/go/main/App.js'
  import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime'
  import type { providers, store } from '../wailsjs/go/models'
  import { starredItems } from './lib/starred'
  import { matchesPanel } from './lib/panels'

  let view: 'tasks' | 'watchlist' | 'settings' | 'panel' = 'tasks'
  let allTasks: providers.TaskItem[] = []
  let taskCount = 0

  let panels: store.Panel[] = []
  let activePanelId: string | null = null
  let showPanelForm = false
  let editingPanel: store.Panel | null = null

  $: activePanel = panels.find((p) => p.id === activePanelId) ?? null
  $: panelCounts = Object.fromEntries(panels.map((p) => [p.id, allTasks.filter((t) => matchesPanel(t, p)).length]))

  // Matches Seguimiento's default view (closed items hidden), so the badge
  // doesn't count entries the user won't actually see there.
  $: watchCount = Object.values($starredItems).filter((item) => !item.closed).length

  function onUpdated(items: providers.TaskItem[]) {
    allTasks = items ?? []
    taskCount = allTasks.length
  }

  onMount(async () => {
    EventsOn('navigate', (target: string) => {
      if (target === 'tasks' || target === 'watchlist' || target === 'settings') view = target
    })
    EventsOn('tasks:updated', onUpdated)
    try {
      allTasks = (await GetTasks()) ?? []
      taskCount = allTasks.length
    } catch {
      // TaskList surfaces the load error; the sidebar count just stays at 0.
    }
    try {
      panels = (await ListPanels()) ?? []
    } catch {
      // The sidebar just won't show any custom panels this session.
    }
  })

  onDestroy(() => {
    EventsOff('navigate')
    EventsOff('tasks:updated')
  })

  function openNewPanel() {
    editingPanel = null
    showPanelForm = true
  }

  function openEditPanel(p: store.Panel) {
    editingPanel = p
    showPanelForm = true
  }

  function onPanelSaved(e: CustomEvent<store.Panel>) {
    const saved = e.detail
    const idx = panels.findIndex((p) => p.id === saved.id)
    panels = idx === -1 ? [...panels, saved] : panels.map((p) => (p.id === saved.id ? saved : p))
    showPanelForm = false
    view = 'panel'
    activePanelId = saved.id
  }

  function onPanelDeleted(e: CustomEvent<string>) {
    const id = e.detail
    panels = panels.filter((p) => p.id !== id)
    showPanelForm = false
    if (activePanelId === id) {
      view = 'tasks'
      activePanelId = null
    }
  }
</script>

<main>
  <aside>
    <div class="brand">
      <span class="brand-mark">K</span>
      <span class="brand-name">Koalmine</span>
      <button
        class="settings-icon"
        class:active={view === 'settings'}
        title={$_('nav.settings')}
        aria-label={$_('nav.settings')}
        on:click={() => (view = 'settings')}
      >
        <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="3" />
          <path
            d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.6a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"
          />
        </svg>
      </button>
    </div>

    <nav>
      <button class="nav-item" class:active={view === 'tasks'} on:click={() => (view = 'tasks')}>
        <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M4 6h16M4 12h16M4 18h10" stroke-linecap="round" />
        </svg>
        <span>{$_('nav.tasks')}</span>
        {#if taskCount > 0}<span class="count">{taskCount}</span>{/if}
      </button>
      <button class="nav-item" class:active={view === 'watchlist'} on:click={() => (view = 'watchlist')}>
        <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M12 17.27 18.18 21l-1.64-7.03L22 9.24l-7.19-.61L12 2 9.19 8.63 2 9.24l5.46 4.73L5.82 21z" stroke-linejoin="round" />
        </svg>
        <span>{$_('nav.watchlist')}</span>
        {#if watchCount > 0}<span class="count">{watchCount}</span>{/if}
      </button>

      {#if panels.length > 0}
        <div class="nav-divider"></div>
      {/if}

      {#each panels as p (p.id)}
        <div class="panel-row">
          <button
            class="nav-item panel-btn"
            class:active={view === 'panel' && activePanelId === p.id}
            on:click={() => {
              view = 'panel'
              activePanelId = p.id
            }}
          >
            <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M4 5h16l-6 7v6l-4 2v-8L4 5z" stroke-linejoin="round" />
            </svg>
            <span class="panel-name">{p.name}</span>
            {#if panelCounts[p.id] > 0}<span class="count">{panelCounts[p.id]}</span>{/if}
          </button>
          <button class="panel-edit" on:click={() => openEditPanel(p)} title={$_('panels.edit')} aria-label={$_('panels.edit')}>
            <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M12 20h9" stroke-linecap="round" />
              <path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4Z" stroke-linejoin="round" />
            </svg>
          </button>
        </div>
      {/each}

      <button class="nav-item add-panel" on:click={openNewPanel}>
        <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M12 5v14M5 12h14" stroke-linecap="round" />
        </svg>
        <span>{$_('panels.add')}</span>
      </button>
    </nav>
  </aside>

  <section class="content">
    {#if view === 'tasks'}
      <TaskList />
    {:else if view === 'watchlist'}
      <TaskList lockToStarred={true} />
    {:else if view === 'panel' && activePanel}
      {#key activePanel.id}
        <TaskList panel={activePanel} />
      {/key}
    {:else if view === 'settings'}
      <Settings />
    {:else}
      <TaskList />
    {/if}
  </section>
</main>

{#if showPanelForm}
  <PanelForm
    panel={editingPanel}
    on:saved={onPanelSaved}
    on:deleted={onPanelDeleted}
    on:close={() => (showPanelForm = false)}
  />
{/if}

<style>
  main {
    height: 100vh;
    display: flex;
    text-align: left;
    background: var(--bg);
  }

  aside {
    width: 210px;
    flex-shrink: 0;
    background: var(--bg-sidebar);
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    padding: 1rem 0.75rem;
    overflow-y: auto;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 0.55rem;
    padding: 0.4rem 0.5rem 1.25rem;
  }

  .settings-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border: none;
    background: transparent;
    color: var(--text-muted);
    border-radius: var(--radius-sm);
    cursor: pointer;
    flex-shrink: 0;
  }

  .settings-icon:hover {
    background: var(--bg-elevated);
    color: var(--text);
  }

  .settings-icon.active {
    background: var(--accent-soft);
    color: var(--accent);
  }

  .brand-mark {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 26px;
    height: 26px;
    border-radius: 7px;
    background: var(--accent);
    color: white;
    font-weight: 800;
    font-size: 0.9rem;
    flex-shrink: 0;
  }

  .brand-name {
    flex: 1;
    font-weight: 700;
    font-size: 1rem;
    letter-spacing: 0.01em;
  }

  nav {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }

  .nav-divider {
    height: 1px;
    background: var(--border);
    margin: 0.5rem 0.4rem;
  }

  .nav-item {
    display: flex;
    align-items: center;
    gap: 0.65rem;
    border: none;
    background: transparent;
    color: var(--text-muted);
    padding: 0.5rem 0.6rem;
    border-radius: var(--radius-sm);
    cursor: pointer;
    font-size: 0.9rem;
    font-family: inherit;
    text-align: left;
    width: 100%;
  }

  .nav-item:hover {
    background: var(--bg-elevated);
    color: var(--text);
  }

  .nav-item.active {
    background: var(--accent-soft);
    color: var(--text);
  }

  .nav-item.active svg {
    color: var(--accent);
  }

  .nav-item span:first-of-type {
    flex: 1;
  }

  .panel-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .add-panel {
    color: var(--text-faint);
  }

  .count {
    font-size: 0.75rem;
    color: var(--text-faint);
    background: var(--bg-elevated);
    border-radius: 999px;
    padding: 0.05rem 0.45rem;
    flex: 0 !important;
  }

  .panel-row {
    display: flex;
    align-items: stretch;
    gap: 0.1rem;
  }

  .panel-row .panel-btn {
    flex: 1;
    min-width: 0;
  }

  .panel-edit {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 26px;
    border: none;
    background: transparent;
    color: var(--text-faint);
    border-radius: var(--radius-sm);
    cursor: pointer;
  }

  .panel-edit:hover {
    background: var(--bg-elevated);
    color: var(--text);
  }

  .content {
    flex: 1;
    overflow-y: auto;
  }
</style>
