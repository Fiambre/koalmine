<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import Settings from './lib/Settings.svelte'
  import TaskList from './lib/TaskList.svelte'
  import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime'

  let view: 'tasks' | 'settings' = 'tasks'

  onMount(() => {
    EventsOn('navigate', (target: string) => {
      if (target === 'tasks' || target === 'settings') view = target
    })
  })

  onDestroy(() => {
    EventsOff('navigate')
  })
</script>

<main>
  <nav>
    <span class="brand">Koalmine</span>
    <button class:active={view === 'tasks'} on:click={() => (view = 'tasks')}>Tareas</button>
    <button class:active={view === 'settings'} on:click={() => (view = 'settings')}>Configuración</button>
  </nav>

  {#if view === 'tasks'}
    <TaskList />
  {:else}
    <Settings />
  {/if}
</main>

<style>
  main {
    height: 100vh;
    display: flex;
    flex-direction: column;
    text-align: left;
  }

  nav {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid rgba(255, 255, 255, 0.1);
  }

  .brand {
    font-weight: 700;
    margin-right: 1rem;
  }

  nav button {
    border: none;
    background: transparent;
    color: rgba(255, 255, 255, 0.7);
    padding: 0.4rem 0.75rem;
    border-radius: 4px;
    cursor: pointer;
  }

  nav button.active {
    background: rgba(255, 255, 255, 0.12);
    color: white;
  }
</style>
