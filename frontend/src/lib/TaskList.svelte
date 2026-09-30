<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { _ } from 'svelte-i18n'
  import { GetTasks, GetPanelAssignedTasks, GetPanelCreatedByMeTasks, RefreshNow, OpenURL, ListIntegrations, CreateTask, SearchTasks, ListProjects, GetComments, RefreshTaskItem } from '../../wailsjs/go/main/App.js'
  import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
  import type { providers, main, store } from '../../wailsjs/go/models'
  import { starredItems, toggleStar, updateStarredItem, markRefreshed, needsRefresh } from './starred'
  import { matchesPanel, matchesPanelFilters } from './panels'

  export let lockToStarred = false
  // A custom panel's filter (project/integration/type/status) — its own
  // dimensions aren't re-exposed as tabs/chips (see the tabs-left guard
  // below), but mineOnly/starredOnly still layer on top of it.
  export let panel: store.Panel | null = null

  // A panel with its own "assigned to" filter is outside the poller's
  // "assigned to me" snapshot (see panels.ts's matchesPanel), so it gets its
  // items from a dedicated fetch instead of the shared tasks/tasks:updated
  // flow below — see loadAssignedTasks. createdByMe takes priority over a
  // leftover/stale assignedTo value (the form hides "Asignado a" once
  // "Creado por mí" is checked and clears it on save, but a panel saved
  // before that existed — or edited outside the form — could still have
  // both set; silently falling back to "everyone assigned" for a panel
  // named "created by me" would be far more confusing than ignoring a
  // redundant assignedTo).
  $: usesAssignedFetch = !panel?.createdByMe && !!panel?.assignedTo
  let assignedTasks: providers.TaskItem[] = []
  let assignedLoading = false
  let assignedError = ''

  async function loadAssignedTasks() {
    if (!panel) return
    assignedLoading = true
    assignedError = ''
    try {
      const items = (await GetPanelAssignedTasks(panel)) ?? []
      assignedTasks = items.filter((t) => matchesPanelFilters(t, panel!))
    } catch (e) {
      assignedError = String(e)
      assignedTasks = []
    } finally {
      assignedLoading = false
      loadedOnce = true
    }
  }

  // A panel with its own "created by me" filter is also outside the
  // poller's snapshot (see panels.ts's matchesPanel) — same reasoning as
  // usesAssignedFetch above, via its own dedicated fetch. Takes priority
  // over assignedTo — see usesAssignedFetch's comment.
  $: usesCreatedByMeFetch = !!panel?.createdByMe
  let createdByMeTasks: providers.TaskItem[] = []
  let createdByMeLoading = false
  let createdByMeError = ''

  async function loadCreatedByMeTasks() {
    if (!panel) return
    createdByMeLoading = true
    createdByMeError = ''
    try {
      const items = (await GetPanelCreatedByMeTasks(panel)) ?? []
      createdByMeTasks = items.filter((t) => matchesPanelFilters(t, panel!))
    } catch (e) {
      createdByMeError = String(e)
      createdByMeTasks = []
    } finally {
      createdByMeLoading = false
      loadedOnce = true
    }
  }

  type Filter = 'all' | 'issue' | 'pr' | 'mention'

  let typeLabels: Record<string, string>
  $: typeLabels = {
    issue: $_('tasks.typeIssue'),
    pr: $_('tasks.typePr'),
    mention: $_('tasks.typeMention'),
  }

  let tasks: providers.TaskItem[] = []
  let filter: Filter = 'all'
  let refreshing = false
  let loadedOnce = false
  let loadError = ''
  let selected: providers.TaskItem | null = null

  let integrationList: main.IntegrationInfo[] = []
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
  // Seguimiento only: a starred item stays starred once closed (see
  // starred.ts), but clutters the list otherwise — hidden by default,
  // this brings closed items back so the user can still find them.
  let showClosed = false

  const VIEW_MODE_KEY = 'koalmine:viewMode'
  function loadViewMode(): 'list' | 'table' {
    try {
      return localStorage.getItem(VIEW_MODE_KEY) === 'table' ? 'table' : 'list'
    } catch {
      return 'list'
    }
  }
  let viewMode: 'list' | 'table' = loadViewMode()
  function setViewMode(mode: 'list' | 'table') {
    viewMode = mode
    try {
      localStorage.setItem(VIEW_MODE_KEY, mode)
    } catch {
      // localStorage unavailable — the choice just won't persist across launches.
    }
  }

  function relativeTime(iso: string): string {
    const date = new Date(iso)
    if (isNaN(date.getTime())) return ''
    const minutes = Math.round((Date.now() - date.getTime()) / 60000)
    if (minutes < 1) return $_('tasks.relativeNow')
    if (minutes < 60) return $_('tasks.relativeMinutes', { values: { n: minutes } })
    const hours = Math.round(minutes / 60)
    if (hours < 24) return $_('tasks.relativeHours', { values: { n: hours } })
    const days = Math.round(hours / 24)
    if (days < 30) return $_('tasks.relativeDays', { values: { n: days } })
    const months = Math.round(days / 30)
    if (months < 12) return $_(months === 1 ? 'tasks.relativeMonth' : 'tasks.relativeMonths', { values: { n: months } })
    const years = Math.round(months / 12)
    return $_(years === 1 ? 'tasks.relativeYear' : 'tasks.relativeYears', { values: { n: years } })
  }

  let comments: providers.Comment[] = []
  let loadingComments = false
  let commentsError = ''
  let commentsLoadedFor: string | null = null

  async function loadComments(item: providers.TaskItem) {
    commentsLoadedFor = item.id
    loadingComments = true
    commentsError = ''
    comments = []
    try {
      comments = (await GetComments(item)) ?? []
    } catch (e) {
      commentsError = String(e)
    } finally {
      loadingComments = false
    }
  }

  // Runs once per newly selected item, regardless of which view (list or
  // table) triggered the selection.
  $: if (selected && selected.id !== commentsLoadedFor) {
    loadComments(selected)
  }

  $: dedicatedLoading = usesAssignedFetch ? assignedLoading : usesCreatedByMeFetch ? createdByMeLoading : refreshing
  $: enabledIntegrations = integrationList.filter((i) => i.enabled)
  $: formProviderInfo = enabledIntegrations.find((i) => i.id === formProvider) ?? null
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
    if (usesAssignedFetch) {
      loadAssignedTasks()
    } else if (usesCreatedByMeFetch) {
      loadCreatedByMeTasks()
    } else {
      EventsOn('tasks:updated', onUpdated)
      try {
        tasks = (await GetTasks()) ?? []
      } catch (e) {
        loadError = String(e)
      } finally {
        loadedOnce = true
      }
    }
    try {
      integrationList = await ListIntegrations()
    } catch {
      // The "new task" form just won't have any integration to offer.
    }
    if (lockToStarred) refreshStarred()
  })

  onDestroy(() => {
    EventsOff('tasks:updated')
  })

  // Starred items outside the regular "assigned to me" poll scope (e.g.
  // created by the user but assigned elsewhere) never get updated by it, and
  // may have been saved with an incomplete snapshot to begin with — see
  // RefreshTaskItem. Two things keep this from turning into a stampede of
  // requests as the favorites list grows: a capped batch size (never more
  // than REFRESH_CONCURRENCY in flight at once) and, unless forced, skipping
  // anything refreshed recently — so switching back to this tab repeatedly
  // doesn't re-fetch everything every time. Best-effort per item either way:
  // one failing (deleted, access revoked, offline) doesn't block the rest.
  const REFRESH_CONCURRENCY = 4
  const AUTO_REFRESH_MAX_AGE_MS = 5 * 60 * 1000

  async function refreshStarred(force = false) {
    const pending = Object.values($starredItems).filter((item) => force || needsRefresh(item.id, AUTO_REFRESH_MAX_AGE_MS))
    for (let i = 0; i < pending.length; i += REFRESH_CONCURRENCY) {
      const batch = pending.slice(i, i + REFRESH_CONCURRENCY)
      await Promise.allSettled(
        batch.map(async (item) => {
          try {
            updateStarredItem(await RefreshTaskItem(item))
          } catch {
            // Keep the existing snapshot rather than surface a per-item error.
          } finally {
            markRefreshed(item.id)
          }
        }),
      )
    }
  }

  async function refresh() {
    if (usesAssignedFetch) {
      await loadAssignedTasks()
      return
    }
    if (usesCreatedByMeFetch) {
      await loadCreatedByMeTasks()
      return
    }
    refreshing = true
    loadError = ''
    if (lockToStarred) refreshStarred(true)
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
    if (!formProvider && enabledIntegrations.length > 0) {
      formProvider = enabledIntegrations[0].id
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
      createError = $_('tasks.formValidation')
      return
    }
    creating = true
    createError = ''
    try {
      const created = await CreateTask({
        integration: formProvider,
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

  // A starred item may not be in the current "assigned to me" poll snapshot
  // (it could've come from a search result, or fallen out of scope since) —
  // prefer the live copy from tasks when there is one, so title/status stay
  // fresh, but fall back to the snapshot saved at star-time otherwise.
  $: starredList = Object.values($starredItems).map((saved) => tasks.find((t) => t.id === saved.id) ?? saved)
  $: baseList = lockToStarred
    ? starredList
    : searchResults ?? (usesAssignedFetch ? assignedTasks : usesCreatedByMeFetch ? createdByMeTasks : tasks)
  $: filtered = baseList
    .filter((t) => filter === 'all' || t.type === filter)
    .filter((t) => !mineOnly || t.createdByMe)
    .filter((t) => !starredOnly || t.id in $starredItems)
    // Closed items are hidden by default wherever the base list can contain
    // them (Seguimiento, and a "created by me" panel — both fetch open and
    // closed alike, unlike every other view) until toggled back on.
    .filter((t) => !(lockToStarred || usesCreatedByMeFetch) || showClosed || !t.closed)
    // Items from a dedicated fetch (assignedTo or createdByMe) are already
    // scoped to the right panel — re-checking them against matchesPanel
    // would wrongly drop every single one, since that function always
    // rejects both kinds of panel unconditionally (see panels.ts).
    .filter((t) => usesAssignedFetch || usesCreatedByMeFetch || !panel || matchesPanel(t, panel))
</script>

<section class="tasks">
  <header>
    <h1>{panel ? panel.name : lockToStarred ? $_('nav.watchlist') : $_('tasks.headerAll')}</h1>
    <div class="search-box">
      <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="11" cy="11" r="7" />
        <path d="m20 20-3.5-3.5" stroke-linecap="round" />
      </svg>
      <input type="search" bind:value={searchQuery} on:input={onSearchInput} placeholder={$_('tasks.searchPlaceholder')} />
      {#if searchQuery}
        <button class="clear-search" on:click={clearSearch} title={$_('tasks.clearSearch')}>×</button>
      {/if}
    </div>
    <div class="header-actions">
      <button class="new-task" on:click={openForm} disabled={enabledIntegrations.length === 0} title={enabledIntegrations.length === 0 ? $_('tasks.newTaskDisabledHint') : $_('tasks.newTask')}>
        <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M12 5v14M5 12h14" stroke-linecap="round" />
        </svg>
        {$_('tasks.newTask')}
      </button>
      <button class="refresh" on:click={refresh} disabled={dedicatedLoading} title={$_('tasks.refresh')}>
        <svg
          class:spin={dedicatedLoading}
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
        {dedicatedLoading ? $_('tasks.refreshing') : $_('tasks.refresh')}
      </button>
    </div>
  </header>

  <div class="tabs">
    <div class="tabs-left">
      {#if !panel?.type}
        {#each [['all', $_('tasks.tabAll')], ['issue', $_('tasks.tabIssue')], ['pr', $_('tasks.tabPr')], ['mention', $_('tasks.tabMention')]] as [value, label] (value)}
          <button class="tab-btn" class:active={filter === value} on:click={() => (filter = value as Filter)}>{label}</button>
        {/each}
      {/if}
    </div>
    <div class="tabs-right">
      <button class="chip" class:active={mineOnly} on:click={() => (mineOnly = !mineOnly)}>{$_('tasks.chipMine')}</button>
      {#if !lockToStarred}
        <button class="chip" class:active={starredOnly} on:click={() => (starredOnly = !starredOnly)}>{$_('tasks.chipStarred')}</button>
      {/if}
      {#if lockToStarred || usesCreatedByMeFetch}
        <button class="chip" class:active={showClosed} on:click={() => (showClosed = !showClosed)}>{$_('tasks.chipShowClosed')}</button>
      {/if}
      <div class="view-toggle">
        <button class="view-btn" class:active={viewMode === 'list'} on:click={() => setViewMode('list')} title={$_('tasks.viewList')}>
          <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M4 6h16M4 12h16M4 18h10" stroke-linecap="round" />
          </svg>
        </button>
        <button class="view-btn" class:active={viewMode === 'table'} on:click={() => setViewMode('table')} title={$_('tasks.viewTable')}>
          <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="3" y="4" width="18" height="16" rx="2" />
            <path d="M3 10h18M9 10v10" />
          </svg>
        </button>
      </div>
    </div>
  </div>

  {#if loadError}
    <p class="status error">{$_('tasks.loadError', { values: { error: loadError } })}</p>
  {/if}

  {#if usesAssignedFetch && assignedError}
    <p class="status error">{$_('tasks.loadError', { values: { error: assignedError } })}</p>
  {/if}

  {#if usesCreatedByMeFetch && createdByMeError}
    <p class="status error">{$_('tasks.loadError', { values: { error: createdByMeError } })}</p>
  {/if}

  {#if searchError}
    <p class="status error">{$_('tasks.searchError', { values: { error: searchError } })}</p>
  {:else if searchResults !== null}
    <p class="search-status">
      {#if searching}
        {$_('tasks.searching')}
      {:else}
        {$_(filtered.length === 1 ? 'tasks.searchResult' : 'tasks.searchResults', { values: { count: filtered.length, query: searchQuery } })}
      {/if}
    </p>
  {/if}

  {#if !loadedOnce}
    <p class="hint">{$_('tasks.loading')}</p>
  {:else if viewMode === 'list'}
    <div class="layout">
      <ul class="list-pane">
        {#if filtered.length === 0}
          <li class="list-empty">
            <p>{@render emptyText()}</p>
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
                  class:active={item.id in $starredItems}
                  on:click={() => toggleStar(item)}
                  title={item.id in $starredItems ? $_('tasks.starRemove') : $_('tasks.starAdd')}
                >
                  <svg viewBox="0 0 24 24" width="15" height="15" fill={item.id in $starredItems ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="2">
                    <path d="M12 17.27 18.18 21l-1.64-7.03L22 9.24l-7.19-.61L12 2 9.19 8.63 2 9.24l5.46 4.73L5.82 21z" stroke-linejoin="round" />
                  </svg>
                </button>
              </div>
            </li>
          {/each}
        {/if}
      </ul>

      <div class="detail-pane">{@render detailContent()}</div>
    </div>
  {:else}
    <div class="table-layout">
      <div class="table-wrap">
        {#if filtered.length === 0}
          <p class="list-empty">{@render emptyText()}</p>
        {:else}
          <table>
            <thead>
              <tr>
                <th class="th-dot"></th>
                <th>{$_('tasks.colTitle')}</th>
                <th>{$_('tasks.colProject')}</th>
                <th>{$_('tasks.colStatus')}</th>
                <th>{$_('tasks.colUpdated')}</th>
                <th class="th-star"></th>
              </tr>
            </thead>
            <tbody>
              {#each filtered as item (item.id)}
                <tr
                  class:selected={!showForm && selected?.id === item.id}
                  on:click={() => {
                    showForm = false
                    selected = item
                  }}
                >
                  <td><span class="dot {item.type}"></span></td>
                  <td class="cell-title">{item.title}</td>
                  <td class="cell-muted cell-project" title={item.project}>{item.project}</td>
                  <td class="cell-muted">{item.status}</td>
                  <td class="cell-muted">{relativeTime(item.updatedAt)}</td>
                  <td>
                    <button
                      class="star-btn"
                      class:active={item.id in $starredItems}
                      on:click|stopPropagation={() => toggleStar(item)}
                      title={item.id in $starredItems ? $_('tasks.starRemove') : $_('tasks.starAdd')}
                    >
                      <svg viewBox="0 0 24 24" width="15" height="15" fill={item.id in $starredItems ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="2">
                        <path d="M12 17.27 18.18 21l-1.64-7.03L22 9.24l-7.19-.61L12 2 9.19 8.63 2 9.24l5.46 4.73L5.82 21z" stroke-linejoin="round" />
                      </svg>
                    </button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}
      </div>

      <div class="table-detail">{@render detailContent()}</div>
    </div>
  {/if}
</section>

{#snippet emptyText()}
  {#if lockToStarred}
    {#if starredList.length > 0}
      {$_('tasks.emptyStarredClosed')}
    {:else}
      {$_('tasks.emptyNoStarred')}
    {/if}
  {:else if tasks.length === 0}
    {$_('tasks.emptyNoTasks')}
  {:else}
    {$_('tasks.emptyNoFilterMatch')}
  {/if}
{/snippet}

{#snippet detailContent()}
  {#if showForm}
    <article class="detail">
      <h2>{$_('tasks.formTitle')}</h2>

      <label class="form-field">
        <span>{$_('tasks.formProvider')}</span>
        <select value={formProvider} on:change={onProviderChange}>
          {#each enabledIntegrations as i (i.id)}
            <option value={i.id}>{i.name}</option>
          {/each}
        </select>
      </label>

      <label class="form-field">
        <span>{$_('tasks.formProject')}{#if formProviderInfo && !showProjectDropdown} — {$_(formProviderInfo.projectHint)}{/if}</span>
        {#if loadingProjects}
          <p class="hint small">{$_('tasks.formProjectLoading')}</p>
        {:else if showProjectDropdown}
          <select bind:value={formProject}>
            <option value="" disabled>{$_('tasks.formProjectChoose')}</option>
            {#each projectOptions as opt (opt.value)}
              <option value={opt.value}>{opt.label}</option>
            {/each}
          </select>
          <button type="button" class="link-btn" on:click={() => { manualProject = true; formProject = '' }}>
            {$_('tasks.formProjectManual')}
          </button>
        {:else}
          {#if projectLoadError}
            <p class="hint small">{$_('tasks.formProjectLoadError')}</p>
          {/if}
          <input type="text" bind:value={formProject} placeholder={formProviderInfo ? $_(formProviderInfo.projectHint) : ''} />
          {#if projectOptions.length > 0}
            <button type="button" class="link-btn" on:click={() => (manualProject = false)}>
              {$_('tasks.formProjectFromList')}
            </button>
          {/if}
        {/if}
      </label>

      <label class="form-field">
        <span>{$_('tasks.formTitleLabel')}</span>
        <input type="text" bind:value={formTitle} placeholder={$_('tasks.formTitlePlaceholder')} />
      </label>

      <label class="form-field">
        <span>{$_('tasks.formDescriptionLabel')}</span>
        <textarea bind:value={formDescription} rows="6" placeholder={$_('tasks.formDescriptionPlaceholder')}></textarea>
      </label>

      {#if createError}
        <p class="status error">{createError}</p>
      {/if}

      <div class="form-actions">
        <button on:click={closeForm} disabled={creating}>{$_('tasks.formCancel')}</button>
        <button class="open-external" on:click={submitForm} disabled={creating}>
          {creating ? $_('tasks.formCreating') : $_('tasks.formCreate')}
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
            class:active={selected.id in $starredItems}
            on:click={() => toggleStar(selected!)}
            title={selected.id in $starredItems ? $_('tasks.starRemove') : $_('tasks.starAdd')}
          >
            <svg viewBox="0 0 24 24" width="16" height="16" fill={selected.id in $starredItems ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="2">
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
          <p class="hint">{$_('tasks.detailNoDescription')}</p>
        {/if}

        <div class="comments">
          <h3>{$_('tasks.detailComments')}</h3>
          {#if loadingComments}
            <p class="hint small">{$_('tasks.detailLoadingComments')}</p>
          {:else if commentsError}
            <p class="status error">{$_('tasks.detailCommentsError', { values: { error: commentsError } })}</p>
          {:else if comments.length === 0}
            <p class="hint small">{$_('tasks.detailNoComments')}</p>
          {:else}
            <ul class="comment-list">
              {#each comments as c, i (i)}
                <li class="comment">
                  <div class="comment-head">
                    <span class="comment-author">{c.author || $_('tasks.detailSomeone')}</span>
                    <span class="comment-date">{relativeTime(c.createdAt)}</span>
                  </div>
                  <p class="comment-body">{c.body}</p>
                </li>
              {/each}
            </ul>
          {/if}
        </div>

        <button class="primary open-external" on:click={() => openExternal(selected!.url)}>
          {$_('tasks.detailOpenExternal')}
          <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M7 17 17 7M9 7h8v8" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </button>
      </article>
    {/key}
  {:else}
    <div class="detail-empty">
      <p>{$_('tasks.detailSelectPrompt')}</p>
    </div>
  {/if}
{/snippet}

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

  .view-toggle {
    display: flex;
    gap: 0.15rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 2px;
    margin-bottom: 0.4rem;
  }

  .view-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    border: none;
    background: transparent;
    color: var(--text-faint);
    cursor: pointer;
    padding: 0.25rem 0.4rem;
    border-radius: 4px;
    font-family: inherit;
  }

  .view-btn:hover {
    color: var(--text);
  }

  .view-btn.active {
    background: var(--accent-soft);
    color: var(--accent);
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

  .table-layout {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-height: 0;
    margin: 0 -2.5rem;
  }

  .table-wrap {
    flex-shrink: 0;
    max-height: 45%;
    overflow-y: auto;
    padding: 0 2.5rem;
    border-bottom: 1px solid var(--border);
  }

  .table-wrap .list-empty {
    padding: 1.5rem 0;
  }

  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.85rem;
  }

  thead th {
    text-align: left;
    font-size: 0.72rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.03em;
    color: var(--text-faint);
    padding: 0.5rem 0.6rem;
    border-bottom: 1px solid var(--border);
    position: sticky;
    top: 0;
    background: var(--bg);
  }

  th.th-dot,
  th.th-star {
    width: 2rem;
  }

  tbody tr {
    cursor: pointer;
  }

  tbody tr:hover {
    background: var(--bg-elevated);
  }

  tbody tr.selected {
    background: var(--accent-soft);
  }

  tbody tr.selected .cell-title {
    color: var(--accent);
  }

  td {
    padding: 0.55rem 0.6rem;
    border-bottom: 1px solid var(--border);
    vertical-align: middle;
  }

  td .dot {
    margin-top: 0;
  }

  td .star-btn {
    padding: 0;
  }

  .cell-title {
    color: var(--text);
    font-weight: 500;
  }

  .cell-muted {
    color: var(--text-faint);
    white-space: nowrap;
  }

  .cell-project {
    max-width: 220px;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .table-detail {
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

  .comments {
    margin: 0 0 1.5rem;
  }

  .comments h3 {
    font-size: 0.85rem;
    font-weight: 700;
    margin: 0 0 0.75rem;
    color: var(--text-muted);
  }

  .comment-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.9rem;
  }

  .comment {
    border-left: 2px solid var(--border);
    padding-left: 0.75rem;
  }

  .comment-head {
    display: flex;
    align-items: baseline;
    gap: 0.5rem;
    margin-bottom: 0.2rem;
  }

  .comment-author {
    font-weight: 600;
    font-size: 0.85rem;
  }

  .comment-date {
    font-size: 0.75rem;
    color: var(--text-faint);
  }

  .comment-body {
    margin: 0;
    font-size: 0.87rem;
    line-height: 1.5;
    color: var(--text);
    white-space: pre-wrap;
    word-wrap: break-word;
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
