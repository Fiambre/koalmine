<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { GetTasks, RefreshNow, OpenURL, ListProviders, CreateTask } from '../../wailsjs/go/main/App.js'
  import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
  import type { providers, main } from '../../wailsjs/go/models'

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
  let selected: providers.TaskItem | null = null

  let providerList: main.ProviderInfo[] = []
  let showForm = false
  let formProvider = ''
  let formProject = ''
  let formTitle = ''
  let formDescription = ''
  let creating = false
  let createError = ''

  $: enabledProviders = providerList.filter((p) => p.enabled)
  $: formProviderInfo = enabledProviders.find((p) => p.name === formProvider) ?? null

  function onUpdated(items: providers.TaskItem[]) {
    tasks = items ?? []
    loadedOnce = true
    refreshing = false
    if (selected && !tasks.some((t) => t.id === selected!.id)) {
      selected = null
    }
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
    try {
      providerList = await ListProviders()
    } catch {
      // The "new task" form just won't have any provider to offer.
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

  function openExternal(url: string) {
    OpenURL(url)
  }

  function openForm() {
    if (!formProvider && enabledProviders.length > 0) {
      formProvider = enabledProviders[0].name
    }
    createError = ''
    selected = null
    showForm = true
  }

  function closeForm() {
    showForm = false
    formProject = ''
    formTitle = ''
    formDescription = ''
    createError = ''
  }

  async function submitForm() {
    if (!formProvider || !formProject.trim() || !formTitle.trim()) {
      createError = 'Completá proveedor, proyecto y título.'
      return
    }
    creating = true
    createError = ''
    try {
      const created = await CreateTask({
        provider: formProvider,
        project: formProject.trim(),
        title: formTitle.trim(),
        description: formDescription.trim(),
      } as main.CreateTaskInput)
      tasks = [created, ...tasks.filter((t) => t.id !== created.id)]
      selected = created
      closeForm()
    } catch (e) {
      createError = String(e)
    } finally {
      creating = false
    }
  }

  $: filtered = filter === 'all' ? tasks : tasks.filter((t) => t.type === filter)
</script>

<section class="tasks">
  <header>
    <h1>Todas las tareas</h1>
    <div class="header-actions">
      <button class="new-task" on:click={openForm} disabled={enabledProviders.length === 0} title={enabledProviders.length === 0 ? 'Configurá un proveedor primero' : 'Nueva tarea'}>
        <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M12 5v14M5 12h14" stroke-linecap="round" />
        </svg>
        Nueva tarea
      </button>
      <button class="refresh" on:click={refresh} disabled={refreshing} title="Actualizar">
        <svg
          class:spin={refreshing}
          viewBox="0 0 24 24"
          width="16"
          height="16"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
        >
          <path d="M20 11A8 8 0 0 0 6.35 6.35M4 13a8 8 0 0 0 13.65 4.65" stroke-linecap="round" />
          <path d="M4 4v6h6M20 20v-6h-6" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
        {refreshing ? 'Actualizando…' : 'Actualizar'}
      </button>
    </div>
  </header>

  <div class="tabs">
    {#each [['all', 'Todas'], ['issue', 'Issues'], ['pr', 'PRs'], ['mention', 'Menciones']] as [value, label] (value)}
      <button class:active={filter === value} on:click={() => (filter = value as Filter)}>{label}</button>
    {/each}
  </div>

  {#if loadError}
    <p class="status error">No se pudieron cargar las tareas: {loadError}</p>
  {/if}

  {#if !loadedOnce}
    <p class="hint">Cargando…</p>
  {:else}
    <div class="layout">
      <ul class="list-pane">
        {#if filtered.length === 0}
          <li class="list-empty">
            <p>{tasks.length === 0 ? 'Todavía no hay tareas para mostrar.' : 'Nada en este filtro.'}</p>
          </li>
        {:else}
          {#each filtered as item (item.id)}
            <li>
              <button
                class="task"
                class:selected={!showForm && selected?.id === item.id}
                on:click={() => {
                  showForm = false
                  selected = item
                }}
              >
                <span class="dot {item.type}"></span>
                <span class="title">{item.title}</span>
                <span class="meta">{item.project}</span>
              </button>
            </li>
          {/each}
        {/if}
      </ul>

      <div class="detail-pane">
        {#if showForm}
          <article class="detail">
            <h2>Nueva tarea</h2>

            <label class="form-field">
              <span>Proveedor</span>
              <select bind:value={formProvider}>
                {#each enabledProviders as p (p.name)}
                  <option value={p.name}>{p.displayName}</option>
                {/each}
              </select>
            </label>

            <label class="form-field">
              <span>Proyecto{#if formProviderInfo} — {formProviderInfo.projectHint}{/if}</span>
              <input type="text" bind:value={formProject} placeholder={formProviderInfo?.projectHint ?? ''} />
            </label>

            <label class="form-field">
              <span>Título</span>
              <input type="text" bind:value={formTitle} placeholder="¿Qué hay que hacer?" />
            </label>

            <label class="form-field">
              <span>Descripción</span>
              <textarea bind:value={formDescription} rows="6" placeholder="Detalle opcional"></textarea>
            </label>

            {#if createError}
              <p class="status error">{createError}</p>
            {/if}

            <div class="form-actions">
              <button on:click={closeForm} disabled={creating}>Cancelar</button>
              <button class="open-external" on:click={submitForm} disabled={creating}>
                {creating ? 'Creando…' : 'Crear tarea'}
              </button>
            </div>
          </article>
        {:else if selected}
          {#key selected.id}
            <article class="detail">
              <div class="detail-top">
                <span class="badge {selected.type}">{typeLabels[selected.type] ?? selected.type}</span>
                <span class="detail-status">{selected.status}</span>
              </div>
              <h2>{selected.title}</h2>
              <p class="detail-meta">
                {selected.project} · {selected.provider}
                {#if selected.author}· {selected.author}{/if}
              </p>

              {#if selected.description}
                <pre class="description">{selected.description}</pre>
              {:else}
                <p class="hint">Sin descripción.</p>
              {/if}

              <button class="primary open-external" on:click={() => openExternal(selected!.url)}>
                Abrir en el navegador
                <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M7 17 17 7M9 7h8v8" stroke-linecap="round" stroke-linejoin="round" />
                </svg>
              </button>
            </article>
          {/key}
        {:else}
          <div class="detail-empty">
            <p>Seleccioná una tarea para ver el detalle.</p>
          </div>
        {/if}
      </div>
    </div>
  {/if}
</section>

<style>
  .tasks {
    display: flex;
    flex-direction: column;
    height: 100%;
    padding: 2rem 2.5rem 0;
  }

  header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 1.25rem;
    gap: 1rem;
    flex-shrink: 0;
  }

  h1 {
    font-size: 1.4rem;
    font-weight: 700;
    margin: 0;
  }

  .header-actions {
    display: flex;
    gap: 0.6rem;
  }

  .status.error {
    color: #ff8a8a;
    font-size: 0.85rem;
    margin: 0 0 0.75rem;
  }

  .tabs {
    display: flex;
    gap: 1.25rem;
    border-bottom: 1px solid var(--border);
    margin-bottom: 0;
    flex-shrink: 0;
  }

  .tabs button {
    border: none;
    background: transparent;
    color: var(--text-muted);
    padding: 0.1rem 0 0.7rem;
    cursor: pointer;
    font-size: 0.88rem;
    font-family: inherit;
    border-bottom: 2px solid transparent;
    margin-bottom: -1px;
  }

  .tabs button:hover {
    color: var(--text);
  }

  .tabs button.active {
    color: var(--text);
    border-bottom-color: var(--accent);
    font-weight: 600;
  }

  .refresh,
  .new-task {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 0.4rem 0.85rem;
    cursor: pointer;
    background: transparent;
    color: var(--text);
    font-family: inherit;
    font-size: 0.85rem;
  }

  .new-task {
    background: var(--accent);
    border-color: var(--accent);
    color: white;
  }

  .new-task:hover:not(:disabled) {
    background: var(--accent-hover);
  }

  .refresh:hover:not(:disabled) {
    background: var(--bg-elevated);
  }

  .refresh:disabled,
  .new-task:disabled {
    opacity: 0.6;
    cursor: default;
  }

  .refresh svg.spin {
    animation: spin 0.9s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .hint {
    color: var(--text-faint);
  }

  .layout {
    flex: 1;
    display: flex;
    min-height: 0;
    margin: 0 -2.5rem;
  }

  .list-pane {
    width: 340px;
    flex-shrink: 0;
    list-style: none;
    margin: 0;
    padding: 0.5rem 1.25rem 1.5rem;
    overflow-y: auto;
    border-right: 1px solid var(--border);
  }

  .list-empty {
    color: var(--text-muted);
    padding: 1.5rem 0.6rem;
    font-size: 0.85rem;
  }

  .task {
    width: 100%;
    display: flex;
    align-items: center;
    gap: 0.6rem;
    text-align: left;
    background: transparent;
    border: none;
    border-radius: var(--radius-sm);
    padding: 0.6rem 0.6rem;
    cursor: pointer;
    color: var(--text);
    font-family: inherit;
    font-size: 0.88rem;
  }

  .task:hover {
    background: var(--bg-elevated);
  }

  .task.selected {
    background: var(--accent-soft);
  }

  .task.selected .title {
    color: var(--accent);
  }

  .dot {
    width: 9px;
    height: 9px;
    border-radius: 50%;
    flex-shrink: 0;
    background: var(--text-faint);
  }

  .dot.issue {
    background: #4a90e2;
  }

  .dot.pr {
    background: #9b59b6;
  }

  .dot.mention {
    background: #e58b2e;
  }

  .title {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .meta {
    font-size: 0.75rem;
    color: var(--text-faint);
    flex-shrink: 0;
    max-width: 100px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .detail-pane {
    flex: 1;
    overflow-y: auto;
    padding: 1.75rem 2.5rem 2rem;
  }

  .detail-empty {
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-faint);
  }

  .detail-top {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    margin-bottom: 0.6rem;
  }

  .badge {
    font-size: 0.7rem;
    padding: 0.15rem 0.5rem;
    border-radius: 999px;
    background: var(--bg-elevated-hover);
    color: var(--text-muted);
  }

  .detail-status {
    font-size: 0.78rem;
    color: var(--text-faint);
  }

  .detail h2 {
    margin: 0 0 0.35rem;
    font-size: 1.2rem;
    font-weight: 700;
  }

  .detail-meta {
    margin: 0 0 1.25rem;
    font-size: 0.82rem;
    color: var(--text-faint);
  }

  .description {
    white-space: pre-wrap;
    word-wrap: break-word;
    font-family: inherit;
    font-size: 0.9rem;
    line-height: 1.55;
    color: var(--text);
    margin: 0 0 1.5rem;
  }

  .form-field {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    font-size: 0.85rem;
    color: var(--text-muted);
    margin-bottom: 0.9rem;
  }

  .form-field input,
  .form-field select,
  .form-field textarea {
    padding: 0.45rem 0.6rem;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--text);
    font-family: inherit;
    font-size: 0.9rem;
    resize: vertical;
  }

  .form-field input:focus,
  .form-field select:focus,
  .form-field textarea:focus {
    outline: none;
    border-color: var(--accent);
  }

  .form-actions {
    display: flex;
    gap: 0.6rem;
    margin-top: 0.5rem;
  }

  .form-actions button:first-child {
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 0.5rem 1rem;
    cursor: pointer;
    background: transparent;
    color: var(--text);
    font-family: inherit;
    font-size: 0.85rem;
  }

  .form-actions button:first-child:hover:not(:disabled) {
    background: var(--bg-elevated);
  }

  .open-external {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    border: none;
    border-radius: var(--radius-sm);
    padding: 0.5rem 1rem;
    cursor: pointer;
    background: var(--accent);
    color: white;
    font-family: inherit;
    font-size: 0.85rem;
  }

  .open-external:hover:not(:disabled) {
    background: var(--accent-hover);
  }

  .open-external:disabled {
    opacity: 0.6;
    cursor: default;
  }
</style>
