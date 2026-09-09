<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import Settings from './lib/Settings.svelte'
  import TaskList from './lib/TaskList.svelte'
  import { GetTasks } from '../wailsjs/go/main/App.js'
  import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime'
  import type { providers } from '../wailsjs/go/models'

  let view: 'tasks' | 'settings' = 'tasks'
  let taskCount = 0

  function onUpdated(items: providers.TaskItem[]) {
    taskCount = items?.length ?? 0
  }

  onMount(async () => {
    EventsOn('navigate', (target: string) => {
      if (target === 'tasks' || target === 'settings') view = target
    })
    EventsOn('tasks:updated', onUpdated)
    try {
      taskCount = (await GetTasks())?.length ?? 0
    } catch {
      // TaskList surfaces the load error; the sidebar count just stays at 0.
    }
  })

  onDestroy(() => {
    EventsOff('navigate')
    EventsOff('tasks:updated')
  })
</script>

<main>
  <aside>
    <div class="brand">
      <span class="brand-mark">K</span>
      <span class="brand-name">Koalmine</span>
    </div>

    <nav>
      <button class:active={view === 'tasks'} on:click={() => (view = 'tasks')}>
        <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M4 6h16M4 12h16M4 18h10" stroke-linecap="round" />
        </svg>
        <span>Tareas</span>
        {#if taskCount > 0}<span class="count">{taskCount}</span>{/if}
      </button>
      <button class:active={view === 'settings'} on:click={() => (view = 'settings')}>
        <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="3" />
          <path
            d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.6a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"
          />
        </svg>
        <span>Configuración</span>
      </button>
    </nav>
  </aside>

  <section class="content">
    {#if view === 'tasks'}
      <TaskList />
    {:else}
      <Settings />
    {/if}
  </section>
</main>

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
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 0.55rem;
    padding: 0.4rem 0.5rem 1.25rem;
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
    font-weight: 700;
    font-size: 1rem;
    letter-spacing: 0.01em;
  }

  nav {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }

  nav button {
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
  }

  nav button:hover {
    background: var(--bg-elevated);
    color: var(--text);
  }

  nav button.active {
    background: var(--accent-soft);
    color: var(--text);
  }

  nav button.active svg {
    color: var(--accent);
  }

  nav button span:first-of-type {
    flex: 1;
  }

  .count {
    font-size: 0.75rem;
    color: var(--text-faint);
    background: var(--bg-elevated);
    border-radius: 999px;
    padding: 0.05rem 0.45rem;
    flex: 0 !important;
  }

  .content {
    flex: 1;
    overflow-y: auto;
  }
</style>
