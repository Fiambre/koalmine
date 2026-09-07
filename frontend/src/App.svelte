<script lang="ts">
  import Settings from './lib/Settings.svelte'

  let view: 'tasks' | 'settings' = 'tasks'
</script>

<main>
  <nav>
    <span class="brand">Koalmine</span>
    <button class:active={view === 'tasks'} on:click={() => (view = 'tasks')}>Tareas</button>
    <button class:active={view === 'settings'} on:click={() => (view = 'settings')}>Configuración</button>
  </nav>

  {#if view === 'tasks'}
    <section class="empty-state">
      <p>Todavía no hay tareas para mostrar.</p>
      <p class="hint">Configurá al menos un proveedor para empezar a ver tus tareas acá.</p>
      <button class="primary" on:click={() => (view = 'settings')}>Ir a Configuración</button>
    </section>
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

  .empty-state {
    text-align: center;
    margin: auto;
    padding: 2rem;
  }

  .hint {
    opacity: 0.7;
  }

  .empty-state button.primary {
    border: none;
    border-radius: 4px;
    padding: 0.5rem 1rem;
    cursor: pointer;
    background: #3d7bfd;
    color: white;
    margin-top: 0.5rem;
  }
</style>
