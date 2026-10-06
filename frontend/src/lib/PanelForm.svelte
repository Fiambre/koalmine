<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import { _ } from 'svelte-i18n'
  import { ListIntegrations, ListProjects, ListAssignableUsers, SavePanel, DeletePanel } from '../../wailsjs/go/main/App.js'
  import type { main, providers, store } from '../../wailsjs/go/models'

  // null means "create a new panel"; otherwise the panel being edited.
  export let panel: store.Panel | null = null

  const dispatch = createEventDispatcher<{
    saved: store.Panel
    deleted: string
    close: void
  }>()

  let integrations: main.IntegrationInfo[] = []
  let name = panel?.name ?? ''
  let integrationId = panel?.integrationId ?? ''
  let project = panel?.project ?? ''
  let type = panel?.type ?? ''
  let status = panel?.status ?? ''
  let assignedTo = panel?.assignedTo ?? ''
  let createdByMe = panel?.createdByMe ?? false

  let projectOptions: providers.ProjectOption[] = []
  let loadingProjects = false

  // "Asignado a" needs one specific integration (cross-provider user IDs
  // don't correspond to each other) and, for a usable dropdown instead of a
  // raw ID/username, a project — see providers.Provider.ListAssignableUsers.
  // Hidden once "Creado por mí" is checked: TaskList.svelte gives
  // createdByMe priority over a stale assignedTo value, but showing a field
  // that would silently be ignored is worse than not showing it at all.
  $: selectedIntegration = integrations.find((i) => i.id === integrationId) ?? null
  $: showAssignedToField = !!integrationId && !!selectedIntegration?.supportsAssignedTo && !createdByMe

  // The sentinel Panel.AssignedTo value meaning "every assignee, no
  // restriction" — mirrors providers.AssignedToAll on the Go side.
  const ASSIGNED_TO_ALL = '*'

  let assignedUserOptions: providers.UserOption[] = []
  let loadingAssignedUsers = false
  // "A mí" and "Todos" are always valid dropdown choices regardless of
  // project, so the dropdown starts as the default — manual entry is only
  // the starting point when editing a panel whose saved value is a raw
  // ID/username that isn't one of those two (it may still turn out to match
  // a project member once options load, but there's no need to force that).
  let manualAssignedTo = !!assignedTo && assignedTo !== ASSIGNED_TO_ALL
  let assignedToTimer: ReturnType<typeof setTimeout> | null = null

  function scheduleLoadAssignedUsers() {
    if (assignedToTimer) clearTimeout(assignedToTimer)
    assignedToTimer = setTimeout(loadAssignedUserOptions, 400)
  }

  async function loadAssignedUserOptions() {
    const proj = project.trim()
    if (!integrationId || !proj) {
      assignedUserOptions = []
      return
    }
    loadingAssignedUsers = true
    try {
      assignedUserOptions = (await ListAssignableUsers(integrationId, proj)) ?? []
    } catch {
      // "A mí"/"Todos" still work either way — only the specific-person
      // options are lost, same as the project dropdown's own fallback.
      assignedUserOptions = []
    } finally {
      loadingAssignedUsers = false
    }
  }

  // Re-runs whenever integrationId or project changes, whether from the
  // dropdowns above or typed manually — covers both this form's own fields
  // changing and the initial load when editing an existing panel.
  $: {
    integrationId
    project
    scheduleLoadAssignedUsers()
  }

  let saving = false
  let deleting = false
  let error = ''

  async function loadIntegrations() {
    try {
      integrations = ((await ListIntegrations()) ?? []).filter((i) => i.enabled)
    } catch {
      // The integration dropdown just falls back to "all integrations".
    }
    if (integrationId) loadProjectOptions(integrationId)
  }
  loadIntegrations()

  async function loadProjectOptions(id: string) {
    if (!id) {
      projectOptions = []
      return
    }
    loadingProjects = true
    try {
      projectOptions = (await ListProjects(id)) ?? []
    } catch {
      // The project field just falls back to plain free-text entry.
      projectOptions = []
    } finally {
      loadingProjects = false
    }
  }

  function onIntegrationChange(e: Event) {
    integrationId = (e.target as HTMLSelectElement).value
    loadProjectOptions(integrationId)
  }

  async function submit() {
    if (!name.trim()) {
      error = $_('panels.nameValidation')
      return
    }
    saving = true
    error = ''
    try {
      const saved = await SavePanel({
        id: panel?.id ?? '',
        name: name.trim(),
        integrationId,
        project: project.trim(),
        type,
        status,
        // Cleared when the field isn't showing (no integration chosen, or
        // its provider doesn't support it) so switching away from a
        // previously-assigned integration doesn't silently keep a stale value.
        assignedTo: showAssignedToField ? assignedTo.trim() : '',
        createdByMe,
      } as store.Panel)
      dispatch('saved', saved)
    } catch (e) {
      error = $_('panels.saveError', { values: { error: String(e) } })
    } finally {
      saving = false
    }
  }

  async function remove() {
    if (!panel) return
    if (!confirm($_('panels.deleteConfirm', { values: { name: panel.name } }))) return
    deleting = true
    error = ''
    try {
      await DeletePanel(panel.id)
      dispatch('deleted', panel.id)
    } catch (e) {
      error = $_('panels.deleteError', { values: { error: String(e) } })
      deleting = false
    }
  }
</script>

<div
  class="overlay"
  role="presentation"
  on:click={() => dispatch('close')}
  on:keydown={(e) => e.key === 'Escape' && dispatch('close')}
>
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="dialog" role="dialog" aria-modal="true" tabindex="-1" on:click|stopPropagation>
    <h2>{panel ? $_('panels.formTitleEdit') : $_('panels.formTitleCreate')}</h2>

    <label class="form-field">
      <span>{$_('panels.nameLabel')}</span>
      <input type="text" bind:value={name} placeholder={$_('panels.namePlaceholder')} />
    </label>

    <label class="form-field">
      <span>{$_('panels.integrationLabel')}</span>
      <select value={integrationId} on:change={onIntegrationChange}>
        <option value="">{$_('panels.integrationAll')}</option>
        {#each integrations as i (i.id)}
          <option value={i.id}>{i.name}</option>
        {/each}
      </select>
    </label>

    <label class="form-field">
      <span>{$_('panels.projectLabel')}</span>
      <input type="text" list="panel-project-options" bind:value={project} placeholder={$_('panels.projectAll')} />
      <datalist id="panel-project-options">
        {#each projectOptions as opt (opt.value)}
          <option value={opt.value} label={opt.label}>{opt.label}</option>
        {/each}
      </datalist>
      {#if loadingProjects}
        <p class="hint small">{$_('panels.projectLoading')}</p>
      {/if}
    </label>

    <label class="form-field">
      <span>{$_('panels.typeLabel')}</span>
      <select bind:value={type}>
        <option value="">{$_('panels.typeAll')}</option>
        <option value="issue">{$_('tasks.tabIssue')}</option>
        <option value="pr">{$_('tasks.tabPr')}</option>
        <option value="mention">{$_('tasks.tabMention')}</option>
      </select>
    </label>

    <label class="form-field">
      <span>{$_('panels.statusLabel')}</span>
      <select bind:value={status}>
        <option value="">{$_('panels.statusAll')}</option>
        <option value="open">{$_('panels.statusOpen')}</option>
        <option value="closed">{$_('panels.statusClosed')}</option>
      </select>
    </label>

    {#if showAssignedToField}
      <label class="form-field">
        <span>{$_('panels.assignedToLabel')}</span>
        {#if manualAssignedTo}
          <input type="text" bind:value={assignedTo} placeholder={$_('panels.assignedToPlaceholder')} />
          <button type="button" class="link-btn" on:click={() => (manualAssignedTo = false)}>
            {$_('panels.assignedToFromList')}
          </button>
        {:else}
          <select bind:value={assignedTo}>
            <option value="">{$_('panels.assignedToMe')}</option>
            <option value={ASSIGNED_TO_ALL}>{$_('panels.assignedToAll')}</option>
            {#each assignedUserOptions as opt (opt.value)}
              <option value={opt.value}>{opt.label}</option>
            {/each}
          </select>
          {#if loadingAssignedUsers}
            <p class="hint small">{$_('panels.assignedToLoading')}</p>
          {:else if !project.trim()}
            <p class="hint small">{$_('panels.assignedToNeedsProject')}</p>
          {/if}
          <button type="button" class="link-btn" on:click={() => (manualAssignedTo = true)}>
            {$_('panels.assignedToManual')}
          </button>
        {/if}
      </label>
    {:else if !integrationId && !createdByMe}
      <p class="hint small assigned-hint">{$_('panels.assignedToNeedsIntegration')}</p>
    {/if}

    <label class="form-field checkbox-field">
      <input type="checkbox" bind:checked={createdByMe} />
      <span>{$_('panels.createdByMeLabel')}</span>
    </label>
    {#if createdByMe}
      <p class="hint small">{$_('panels.createdByMeHint')}</p>
    {/if}

    {#if error}
      <p class="status error">{error}</p>
    {/if}

    <div class="form-actions">
      {#if panel}
        <button class="danger" on:click={remove} disabled={saving || deleting}>
          {$_('panels.delete')}
        </button>
      {/if}
      <span class="spacer"></span>
      <button on:click={() => dispatch('close')} disabled={saving || deleting}>{$_('tasks.formCancel')}</button>
      <button class="primary" on:click={submit} disabled={saving || deleting}>
        {saving ? $_('settings.saving') : $_('settings.save')}
      </button>
    </div>
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.45);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 50;
  }

  .dialog {
    width: 360px;
    max-width: calc(100vw - 2rem);
    max-height: calc(100vh - 2rem);
    overflow-y: auto;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 1.5rem;
    box-shadow: 0 12px 32px rgba(0, 0, 0, 0.35);
  }

  h2 {
    margin: 0 0 1.1rem;
    font-size: 1.1rem;
    font-weight: 700;
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
  .form-field select {
    padding: 0.45rem 0.6rem;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--text);
    font-family: inherit;
    font-size: 0.9rem;
  }

  .form-field input:focus,
  .form-field select:focus {
    outline: none;
    border-color: var(--accent);
  }

  .checkbox-field {
    flex-direction: row;
    align-items: center;
    gap: 0.5rem;
  }

  .checkbox-field input {
    width: auto;
  }

  .hint.small {
    font-size: 0.8rem;
    margin: 0;
    color: var(--text-faint);
  }

  .assigned-hint {
    margin-bottom: 0.9rem;
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

  .status.error {
    color: #ff8a8a;
    font-size: 0.85rem;
    margin: 0 0 0.75rem;
  }

  .form-actions {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    margin-top: 0.5rem;
  }

  .spacer {
    flex: 1;
  }

  .form-actions button {
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 0.5rem 1rem;
    cursor: pointer;
    background: transparent;
    color: var(--text);
    font-family: inherit;
    font-size: 0.85rem;
  }

  .form-actions button:hover:not(:disabled) {
    background: var(--bg-elevated);
  }

  .form-actions button.primary {
    border-color: var(--accent);
    background: var(--accent);
    color: white;
  }

  .form-actions button.primary:hover:not(:disabled) {
    background: var(--accent-hover);
  }

  .form-actions button.danger {
    border-color: transparent;
    color: #ff8a8a;
  }

  .form-actions button.danger:hover:not(:disabled) {
    background: rgba(255, 138, 138, 0.1);
  }

  .form-actions button:disabled {
    opacity: 0.6;
    cursor: default;
  }
</style>
