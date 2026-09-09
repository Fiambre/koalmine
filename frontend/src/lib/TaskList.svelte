<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { GetTasks, RefreshNow, OpenURL, ListProviders, CreateTask, SearchTasks, ListProjects } from '../../wailsjs/go/main/App.js'
  import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
  import type { providers, main } from '../../wailsjs/go/models'
  import { starredIds, toggleStar } from './starred'

  export let lockToStarred = false

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
  let projectOptions: providers.ProjectOption[] = []
  let loadingProjects = false
  let projectLoadError = ''
  let manualProject = false

  let searchQuery = ''
  let searching = false
  let searchError = ''
  let searchResults: providers.TaskItem[] | null = null
  let searchTimer: ReturnType<typeof setTimeout> | null = null

  let mineOnly = false
  let starredOnly = lockToStarred

  $: enabledProviders = providerList.filter((p) => p.enabled)
  $: formProviderInfo = enabledProviders.find((p) => p.name === formProvider) ?? null
  $: showProjectDropdown = !manualProject && !loadingProjects && projectOptions.length > 0

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

  function onSearchInput() {
    if (searchTimer) clearTimeout(searchTimer)
    const q = searchQuery.trim()
    if (!q) {
      searchResults = null
      searchError = ''
      return
    }
    searchTimer = setTimeout(() => runSearch(q), 400)
  }

  async function runSearch(query: string) {
    searching = true
    searchError = ''
    try {
      searchResults = (await SearchTasks(query)) ?? []
    } catch (e) {
      searchError = String(e)
      searchResults = []
    } finally {
      searching = false
    }
  }

  function clearSearch() {
    if (searchTimer) clearTimeout(searchTimer)
    searchQuery = ''
    searchResults = null
    searchError = ''
  }

  function openForm() {
    if (!formProvider && enabledProviders.length > 0) {
      formProvider = enabledProviders[0].name
    }
    createError = ''
    selected = null
    showForm = true
    manualProject = false
    if (formProvider) {
      loadProjectOptions(formProvider)
    }
  }

  function onProviderChange(e: Event) {
    formProvider = (e.target as HTMLSelectElement).value
    formProject = ''
    manualProject = false
    loadProjectOptions(formProvider)
  }

  async function loadProjectOptions(providerName: string) {
    loadingProjects = true
    projectLoadError = ''
    projectOptions = []
    try {
      projectOptions = (await ListProjects(providerName)) ?? []
    } catch (e) {
      projectLoadError = String(e)
    } finally {
      loadingProjects = false
    }
  }

  function closeForm() {
    showForm = false
    formProject = ''
    formTitle = ''
    formDescription = ''
    createError = ''
    projectOptions = []
    projectLoadError = ''
    manualProject = false
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

  $: baseList = searchResults ?? tasks
  $: filtered = baseList
    .filter((t) => filter === 'all' || t.type === filter)
    .filter((t) => !mineOnly || t.createdByMe)
    .filter((t) => !(starredOnly || lockToStarred) || $starredIds.has(t.id))
</script>

<section class="tasks">
  <header>
    <h1>{lockToStarred ? 'Seguimiento' : 'Todas las tareas'}</h1>
    <div class="search-box">
      <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="11" cy="11" r="7" />
        <path d="m20 20-3.5-3.5" stroke-linecap="round" />
      </svg>
      <input type="search" bind:value={searchQuery} on:input={onSearchInput} placeholder="Buscar tareas…" />
      {#if searchQuery}
        <button class="clear-search" on:click={clearSearch} title="Limpiar búsqueda">×</button>
      {/if}
    </div>
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
    <div class="tabs-left">
      {#each [['all', 'Todas'], ['issue', 'Issues'], ['pr', 'PRs'], ['mention', 'Menciones']] as [value, label] (value)}
        <button class="tab-btn" class:active={filter === value} on:click={() => (filter = value as Filter)}>{label}</button>
      {/each}
    </div>
    <div class="tabs-right">
      <button class="chip" class:active={mineOnly} on:click={() => (mineOnly = !mineOnly)}>Creadas por mí</button>
      {#if !lockToStarred}
        <button class="chip" class:active={starredOnly} on:click={() => (starredOnly = !starredOnly)}>★ Favoritos</button>
      {/if}
    </div>
  </div>

  {#if loadError}
    <p class="status error">No se pudieron cargar las tareas: {loadError}</p>
  {/if}

  {#if searchError}
    <p class="status error">No se pudo buscar: {searchError}</p>
  {:else if searchResults !== null}
    <p class="search-status">
      {#if searching}Buscando…{:else}{filtered.length} resultado{filtered.length === 1 ? '' : 's'} para “{searchQuery}”{/if}
    </p>
  {/if}

  {#if !loadedOnce}
    <p class="hint">Cargando…</p>
  {:else}
    <div class="layout">
      <ul class="list-pane">
        {#if filtered.length === 0}
          <li class="list-empty">
            <p>
              {#if lockToStarred}
                Todavía no marcaste ninguna tarea. Tocá la ★ en una tarea para agregarla acá.
              {:else if tasks.length === 0}
                Todavía no hay tareas para mostrar.
              {:else}
                Nada en este filtro.
              {/if}
            </p>
          </li>
        {:else}
          {#each filtered as item (item.id)}
            <li>
              <div class="task-row" class:selected={!showForm && selected?.id === item.id}>
                <button
                  class="task"
                  on:click={() => {
                    showForm = false
                    selected = item
                  }}
                >
                  <span class="dot {item.type}"></span>
                  <span class="task-text">
                    <span class="title">{item.title}</span>
                    <span class="meta">{item.project}</span>
                  </span>
                </button>
                <button
                  class="star-btn"
                  class:active={$starredIds.has(item.id)}
                  on:click={() => toggleStar(item.id)}
                  title={$starredIds.has(item.id) ? 'Quitar de favoritos' : 'Agregar a favoritos'}
                >
                  <svg viewBox="0 0 24 24" width="15" height="15" fill={$starredIds.has(item.id) ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="2">
                    <path d="M12 17.27 18.18 21l-1.64-7.03L22 9.24l-7.19-.61L12 2 9.19 8.63 2 9.24l5.46 4.73L5.82 21z" stroke-linejoin="round" />
                  </svg>
                </button>
              </div>
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
              <select value={formProvider} on:change={onProviderChange}>
                {#each enabledProviders as p (p.name)}
                  <option value={p.name}>{p.displayName}</option>
                {/each}
              </select>
            </label>

            <label class="form-field">
              <span>Proyecto{#if formProviderInfo && !showProjectDropdown} — {formProviderInfo.projectHint}{/if}</span>
              {#if loadingProjects}
                <p class="hint small">Cargando proyectos…</p>
              {:else if showProjectDropdown}
                <select bind:value={formProject}>
                  <option value="" disabled>Elegí un proyecto…</option>
                  {#each projectOptions as opt (opt.value)}
                    <option value={opt.value}>{opt.label}</option>
                  {/each}
                </select>
                <button type="button" class="link-btn" on:click={() => { manualProject = true; formProject = '' }}>
                  Escribir manualmente
                </button>
              {:else}
                {#if projectLoadError}
                  <p class="hint small">No se pudo cargar la lista de proyectos; escribilo manualmente.</p>
                {/if}
                <input type="text" bind:value={formProject} placeholder={formProviderInfo?.projectHint ?? ''} />
                {#if projectOptions.length > 0}
                  <button type="button" class="link-btn" on:click={() => (manualProject = false)}>
                    Elegir de la lista
                  </button>
                {/if}
              {/if}
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
                {#if selected.status}<span class="detail-status">{selected.status}</span>{/if}
                <button
                  class="star-btn"
                  class:active={$starredIds.has(selected.id)}
                  on:click={() => toggleStar(selected!.id)}
                  title={$starredIds.has(selected.id) ? 'Quitar de favoritos' : 'Agregar a favoritos'}
                >
                  <svg viewBox="0 0 24 24" width="16" height="16" fill={$starredIds.has(selected.id) ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="2">
                    <path d="M12 17.27 18.18 21l-1.64-7.03L22 9.24l-7.19-.61L12 2 9.19 8.63 2 9.24l5.46 4.73L5.82 21z" stroke-linejoin="round" />
                  </svg>
                </button>
              </div>
              <h2>{selected.title}</h2>
              <p class="detail-meta">
                {[selected.project, selected.provider, selected.author].filter(Boolean).join(' · ')}
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
    align-items: center;
    margin-bottom: 1.25rem;
    gap: 1rem;
    flex-shrink: 0;
  }

  h1 {
    font-size: 1.4rem;
    font-weight: 700;
    margin: 0 auto 0 0;
    flex-shrink: 0;
  }

  .header-actions {
    display: flex;
    gap: 0.6rem;
  }

  .search-box {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 0.35rem 0.7rem;
    background: var(--bg-elevated);
    width: 220px;
    color: var(--text-faint);
    flex-shrink: 0;
  }

  .search-box input {
    border: none;
    background: transparent;
    color: var(--text);
    font-family: inherit;
    font-size: 0.85rem;
    outline: none;
    flex: 1;
    min-width: 0;
  }

  .search-box input::-webkit-search-cancel-button {
    display: none;
  }

  .clear-search {
    border: none;
    background: transparent;
    color: var(--text-faint);
    cursor: pointer;
    font-size: 1.1rem;
    line-height: 1;
    padding: 0;
  }

  .clear-search:hover {
    color: var(--text);
  }

  .search-status {
    margin: 0 0 0.75rem;
    font-size: 0.8rem;
    color: var(--text-faint);
  }

  .status.error {
    color: #ff8a8a;
    font-size: 0.85rem;
    margin: 0 0 0.75rem;
  }

  .tabs {
    display: flex;
    justify-content: space-between;
    align-items: flex-end;
    border-bottom: 1px solid var(--border);
    margin-bottom: 0;
    flex-shrink: 0;
  }

  .tabs-left,
  .tabs-right {
    display: flex;
    gap: 0.5rem;
  }

  .tabs-left {
    gap: 1.25rem;
  }

  .tab-btn {
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

  .tab-btn:hover {
    color: var(--text);
  }

  .tab-btn.active {
    color: var(--text);
    border-bottom-color: var(--accent);
    font-weight: 600;
  }

  .chip {
    border: 1px solid var(--border);
    background: transparent;
    color: var(--text-muted);
    padding: 0.2rem 0.65rem;
    margin-bottom: 0.4rem;
    border-radius: 999px;
    cursor: pointer;
    font-size: 0.78rem;
    font-family: inherit;
  }

  .chip:hover {
    color: var(--text);
  }

  .chip.active {
    background: var(--accent-soft);
    border-color: var(--accent);
    color: var(--text);
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

  .hint.small {
    font-size: 0.8rem;
    margin: 0;
  }

  .link-btn {
    align-self: flex-start;
    border: none;
    background: transparent;
    color: var(--accent);
    cursor: pointer;
    padding: 0;
    font-family: inherit;
    font-size: 0.78rem;
    margin-top: 0.3rem;
  }

  .link-btn:hover {
    text-decoration: underline;
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

  .task-row {
    display: flex;
    align-items: stretch;
    border-radius: var(--radius-sm);
  }

  .task-row:hover {
    background: var(--bg-elevated);
  }

  .task-row.selected {
    background: var(--accent-soft);
  }

  .task-row.selected .title {
    color: var(--accent);
  }

  .task {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: flex-start;
    gap: 0.6rem;
    text-align: left;
    background: transparent;
    border: none;
    padding: 0.65rem 0.4rem 0.65rem 0.6rem;
    cursor: pointer;
    color: var(--text);
    font-family: inherit;
    font-size: 0.88rem;
  }

  .star-btn {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    border: none;
    background: transparent;
    cursor: pointer;
    padding: 0 0.6rem;
    color: var(--text-faint);
  }

  .star-btn:hover {
    color: #f5c518;
  }

  .star-btn.active {
    color: #f5c518;
  }

  .dot {
    width: 9px;
    height: 9px;
    border-radius: 50%;
    flex-shrink: 0;
    margin-top: 0.4rem;
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

  .task-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }

  .title {
    white-space: normal;
    overflow-wrap: break-word;
    line-height: 1.35;
  }

  .meta {
    font-size: 0.75rem;
    color: var(--text-faint);
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

  .detail-top .star-btn {
    margin-left: auto;
    padding: 0;
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
