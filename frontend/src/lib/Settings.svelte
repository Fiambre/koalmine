<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { _ } from 'svelte-i18n'
  import {
    ListProviderTypes,
    ListIntegrations,
    CreateIntegration,
    UpdateIntegration,
    DeleteIntegration,
    TestConnection,
    GetAutostartEnabled,
    SetAutostartEnabled,
    GetAppVersion,
    GetUpdateStatus,
    CheckForUpdateNow,
    ApplyUpdate,
  } from '../../wailsjs/go/main/App.js'
  import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
  import type { main, updater } from '../../wailsjs/go/models'
  import { ACCENT_COLORS, loadAccent, saveAccent } from './theme'
  import { SUPPORTED_LOCALES, setLocale, type Locale } from './i18n'
  import { locale } from 'svelte-i18n'

  const LOCALE_LABELS: Record<Locale, string> = { en: 'English', es: 'Español' }

  type Status = { kind: 'idle' | 'testing' | 'ok' | 'error' | 'saving' | 'saved'; message?: string }

  let selectedAccent = loadAccent()

  function selectAccent(hex: string) {
    selectedAccent = hex
    saveAccent(hex)
  }

  let providerTypes: main.ProviderTypeInfo[] = []
  let integrations: main.IntegrationInfo[] = []
  let loading = true
  let loadError = ''

  let autostart = false
  let autostartStatus: Status = { kind: 'idle' }

  let appVersion = ''
  let updateInfo: updater.Info | null = null
  let updateStatus: Status = { kind: 'idle' }
  let checkStatus: Status = { kind: 'idle' }

  // Per-integration draft form state, keyed by integration id.
  let drafts: Record<string, Record<string, string>> = {}
  let nameDrafts: Record<string, string> = {}
  let enabledDrafts: Record<string, boolean> = {}
  let statusByIntegration: Record<string, Status> = {}

  async function load() {
    loading = true
    loadError = ''
    try {
      providerTypes = await ListProviderTypes()
      integrations = await ListIntegrations()
      for (const integ of integrations) {
        drafts[integ.id] = { ...integ.values }
        nameDrafts[integ.id] = integ.name
        enabledDrafts[integ.id] = integ.enabled
        statusByIntegration[integ.id] = { kind: 'idle' }
      }
      autostart = await GetAutostartEnabled()
      appVersion = await GetAppVersion()
      updateInfo = await GetUpdateStatus()
    } catch (e) {
      loadError = String(e)
    } finally {
      loading = false
    }
  }

  onMount(() => {
    load()
    EventsOn('update:available', async () => {
      updateInfo = await GetUpdateStatus()
    })
  })

  onDestroy(() => {
    EventsOff('update:available')
  })

  async function checkNow() {
    checkStatus = { kind: 'testing' }
    try {
      const info = await CheckForUpdateNow()
      updateInfo = info
      checkStatus = info.available
        ? { kind: 'ok', message: $_('settings.versionAvailable', { values: { version: info.version } }) }
        : { kind: 'ok', message: $_('settings.versionUpToDate') }
    } catch (e) {
      checkStatus = { kind: 'error', message: String(e) }
    }
  }

  async function applyUpdate() {
    updateStatus = { kind: 'saving' }
    try {
      await ApplyUpdate()
      // On success the app quits and relaunches itself — nothing left to update here.
    } catch (e) {
      updateStatus = { kind: 'error', message: String(e) }
    }
  }

  async function toggleAutostart() {
    autostartStatus = { kind: 'saving' }
    try {
      await SetAutostartEnabled(autostart)
      autostartStatus = {
        kind: 'saved',
        message: autostart ? $_('settings.autostartEnabled') : $_('settings.autostartDisabled'),
      }
    } catch (e) {
      autostart = !autostart
      autostartStatus = { kind: 'error', message: String(e) }
    }
  }

  async function testConnection(integ: main.IntegrationInfo) {
    statusByIntegration[integ.id] = { kind: 'testing' }
    try {
      await TestConnection(integ.type, integ.id, drafts[integ.id] ?? {})
      statusByIntegration[integ.id] = { kind: 'ok', message: $_('settings.connectionOk') }
    } catch (e) {
      statusByIntegration[integ.id] = { kind: 'error', message: String(e) }
    }
  }

  async function saveIntegration(integ: main.IntegrationInfo) {
    statusByIntegration[integ.id] = { kind: 'saving' }
    try {
      await UpdateIntegration(integ.id, nameDrafts[integ.id] ?? integ.name, enabledDrafts[integ.id] ?? false, drafts[integ.id] ?? {})
      await load()
      statusByIntegration[integ.id] = { kind: 'saved', message: $_('settings.saved') }
    } catch (e) {
      statusByIntegration[integ.id] = { kind: 'error', message: String(e) }
    }
  }

  async function deleteIntegration(integ: main.IntegrationInfo) {
    if (!confirm($_('settings.deleteConfirm', { values: { name: integ.name } }))) return
    try {
      await DeleteIntegration(integ.id)
      await load()
    } catch (e) {
      statusByIntegration[integ.id] = { kind: 'error', message: String(e) }
    }
  }

  // --- Add integration ---

  let showAddForm = false
  let newType = ''
  let newName = ''
  let newDrafts: Record<string, string> = {}
  let newStatus: Status = { kind: 'idle' }
  let creatingIntegration = false

  $: newTypeInfo = providerTypes.find((t) => t.type === newType) ?? null

  function defaultNameFor(type: main.ProviderTypeInfo): string {
    const count = integrations.filter((integ) => integ.type === type.type).length
    return count === 0 ? type.displayName : `${type.displayName} (${count + 1})`
  }

  function openAddForm() {
    showAddForm = true
    newStatus = { kind: 'idle' }
    if (!newType && providerTypes.length > 0) {
      onNewTypeChange(providerTypes[0].type)
    }
  }

  function closeAddForm() {
    showAddForm = false
    newType = ''
    newName = ''
    newDrafts = {}
    newStatus = { kind: 'idle' }
  }

  function onNewTypeChange(type: string) {
    newType = type
    const info = providerTypes.find((t) => t.type === type)
    newDrafts = Object.fromEntries((info?.fields ?? []).map((f) => [f.key, '']))
    newName = info ? defaultNameFor(info) : ''
  }

  async function testNewConnection() {
    newStatus = { kind: 'testing' }
    try {
      await TestConnection(newType, '', newDrafts)
      newStatus = { kind: 'ok', message: $_('settings.connectionOk') }
    } catch (e) {
      newStatus = { kind: 'error', message: String(e) }
    }
  }

  async function createIntegration() {
    creatingIntegration = true
    try {
      await CreateIntegration(newType, newName, newDrafts)
      await load()
      closeAddForm()
    } catch (e) {
      newStatus = { kind: 'error', message: String(e) }
    } finally {
      creatingIntegration = false
    }
  }
</script>

<section class="settings">
  {#if loading}
    <p class="hint">{$_('tasks.loading')}</p>
  {:else if loadError}
    <p class="status error">{$_('settings.loadError', { values: { error: loadError } })}</p>
  {:else}
    <article class="provider-card">
      <h2 class="card-title">{$_('settings.appearanceTitle')}</h2>
      <p class="hint small">{$_('settings.accentColor')}</p>
      <div class="swatches">
        {#each ACCENT_COLORS as color (color.value)}
          <button
            class="swatch"
            class:selected={selectedAccent === color.value}
            style="background: {color.value}"
            title={$_(color.nameKey)}
            aria-label={$_(color.nameKey)}
            on:click={() => selectAccent(color.value)}
          >
            {#if selectedAccent === color.value}
              <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="white" stroke-width="3">
                <path d="M20 6 9 17l-5-5" stroke-linecap="round" stroke-linejoin="round" />
              </svg>
            {/if}
          </button>
        {/each}
      </div>

      <p class="hint small language-label">{$_('settings.languageTitle')}</p>
      <div class="lang-toggle">
        {#each SUPPORTED_LOCALES as code (code)}
          <button class="lang-btn" class:active={$locale === code} on:click={() => setLocale(code)}>
            {LOCALE_LABELS[code]}
          </button>
        {/each}
      </div>
    </article>

    <article class="provider-card">
      <label class="enable-toggle">
        <input type="checkbox" bind:checked={autostart} on:change={toggleAutostart} disabled={autostartStatus.kind === 'saving'} />
        <strong>{$_('settings.autostartLabel')}</strong>
      </label>
      {#if autostartStatus.kind === 'saved'}
        <span class="status ok">{autostartStatus.message}</span>
      {:else if autostartStatus.kind === 'error'}
        <span class="status error">{autostartStatus.message}</span>
      {/if}
    </article>

    <article class="provider-card">
      <div class="version-row">
        <p class="version-line">{$_('settings.versionCurrent', { values: { version: appVersion } })}</p>
        <button on:click={checkNow} disabled={checkStatus.kind === 'testing'}>
          {checkStatus.kind === 'testing' ? $_('settings.versionChecking') : $_('settings.versionCheck')}
        </button>
      </div>
      {#if checkStatus.kind === 'error'}
        <span class="status error">{checkStatus.message}</span>
      {:else if checkStatus.kind === 'ok' && !updateInfo?.available}
        <span class="status ok">{checkStatus.message}</span>
      {/if}
      {#if updateInfo?.available}
        <p class="update-banner">
          {$_('settings.versionNewBanner', { values: { version: updateInfo.version } })}
          <button class="primary" on:click={applyUpdate} disabled={updateStatus.kind === 'saving'}>
            {updateStatus.kind === 'saving' ? $_('settings.versionUpdating') : $_('settings.versionUpdateNow')}
          </button>
        </p>
        {#if updateStatus.kind === 'error'}
          <span class="status error">{updateStatus.message}</span>
        {/if}
      {/if}
    </article>

    <div class="integrations-header">
      <h2>{$_('settings.integrationsTitle')}</h2>
      {#if !showAddForm}
        <button class="primary" on:click={openAddForm} disabled={providerTypes.length === 0}>
          {$_('settings.addIntegration')}
        </button>
      {/if}
    </div>

    {#if showAddForm}
      <article class="provider-card add-card">
        <header>
          <label class="field type-field">
            <span>{$_('settings.typeLabel')}</span>
            <select value={newType} on:change={(e) => onNewTypeChange((e.target as HTMLSelectElement).value)}>
              {#each providerTypes as type (type.type)}
                <option value={type.type}>{type.displayName}</option>
              {/each}
            </select>
          </label>
          <label class="field">
            <span>{$_('settings.nameLabel')}</span>
            <input type="text" autocomplete="off" bind:value={newName} />
          </label>
        </header>

        {#if newTypeInfo}
          <div class="fields">
            {#each newTypeInfo.fields as field (field.key)}
              <label class="field">
                <span>{$_(field.label)}{field.required ? ' *' : ''}</span>
                <input
                  type={field.kind === 'secret' ? 'password' : field.kind === 'url' ? 'url' : 'text'}
                  autocomplete="off"
                  placeholder={field.placeholder}
                  bind:value={newDrafts[field.key]}
                />
              </label>
            {/each}
          </div>
        {/if}

        <footer>
          <button on:click={closeAddForm}>{$_('tasks.formCancel')}</button>
          <button on:click={testNewConnection} disabled={newStatus.kind === 'testing' || !newType}>
            {newStatus.kind === 'testing' ? $_('settings.testing') : $_('settings.testConnection')}
          </button>
          <button class="primary" on:click={createIntegration} disabled={creatingIntegration || !newType}>
            {creatingIntegration ? $_('settings.creatingIntegration') : $_('settings.createIntegration')}
          </button>
          {#if newStatus.kind === 'ok' || newStatus.kind === 'error'}
            <span class="status" class:ok={newStatus.kind === 'ok'} class:error={newStatus.kind === 'error'}>
              {newStatus.message}
            </span>
          {/if}
        </footer>
      </article>
    {/if}

    {#each integrations as integ (integ.id)}
      <article class="provider-card">
        <header class="integration-header">
          <label class="enable-toggle">
            <input type="checkbox" bind:checked={enabledDrafts[integ.id]} />
          </label>
          <input type="text" class="name-input" autocomplete="off" bind:value={nameDrafts[integ.id]} />
          <span class="type-badge">{integ.typeDisplayName}</span>
          <button class="delete-btn" on:click={() => deleteIntegration(integ)} title={$_('settings.deleteIntegration')}>
            {$_('settings.deleteIntegration')}
          </button>
        </header>

        <div class="fields">
          {#each integ.fields as field (field.key)}
            <label class="field">
              <span>{$_(field.label)}{field.required ? ' *' : ''}</span>
              {#if field.kind === 'secret'}
                <input
                  type="password"
                  autocomplete="off"
                  placeholder={integ.secretsSet[field.key] ? $_('settings.secretUnchanged') : field.placeholder}
                  bind:value={drafts[integ.id][field.key]}
                />
              {:else}
                <input
                  type={field.kind === 'url' ? 'url' : 'text'}
                  autocomplete="off"
                  placeholder={field.placeholder}
                  bind:value={drafts[integ.id][field.key]}
                />
              {/if}
            </label>
          {/each}
        </div>

        <footer>
          <button on:click={() => testConnection(integ)} disabled={statusByIntegration[integ.id]?.kind === 'testing'}>
            {statusByIntegration[integ.id]?.kind === 'testing' ? $_('settings.testing') : $_('settings.testConnection')}
          </button>
          <button class="primary" on:click={() => saveIntegration(integ)} disabled={statusByIntegration[integ.id]?.kind === 'saving'}>
            {statusByIntegration[integ.id]?.kind === 'saving' ? $_('settings.saving') : $_('settings.save')}
          </button>
          {#if statusByIntegration[integ.id]?.kind === 'ok' || statusByIntegration[integ.id]?.kind === 'saved'}
            <span class="status ok">{statusByIntegration[integ.id]?.message}</span>
          {:else if statusByIntegration[integ.id]?.kind === 'error'}
            <span class="status error">{statusByIntegration[integ.id]?.message}</span>
          {/if}
        </footer>
      </article>
    {/each}
  {/if}
</section>

<style>
  .settings {
    text-align: left;
    max-width: 640px;
    margin: 0 auto;
    padding: 2rem 2.5rem;
  }

  h2 {
    margin: 1.5rem 0 0.75rem;
    font-size: 1.1rem;
    font-weight: 700;
  }

  .card-title {
    margin: 0 0 0.5rem;
    font-size: 0.95rem;
    font-weight: 700;
  }

  .hint {
    color: var(--text-faint);
  }

  .hint.small {
    font-size: 0.8rem;
    margin: 0 0 0.6rem;
  }

  .version-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
  }

  .version-line {
    margin: 0;
    font-size: 0.85rem;
    color: var(--text-muted);
  }

  .update-banner {
    margin: 0.5rem 0 0;
    display: flex;
    align-items: center;
    gap: 0.75rem;
    font-size: 0.9rem;
  }

  .provider-card {
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 1.1rem;
    margin-bottom: 1rem;
  }

  .integrations-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
  }

  .integrations-header h2 {
    margin: 1.5rem 0 0.75rem;
  }

  .add-card header {
    display: flex;
    gap: 0.9rem;
    flex-wrap: wrap;
  }

  .type-field select {
    padding: 0.45rem 0.6rem;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--text);
    font-family: inherit;
    font-size: 0.9rem;
  }

  .integration-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }

  .integration-header .enable-toggle {
    flex-shrink: 0;
  }

  .name-input {
    flex: 1;
    min-width: 0;
    border: 1px solid transparent;
    background: transparent;
    color: var(--text);
    font-family: inherit;
    font-size: 0.95rem;
    font-weight: 700;
    padding: 0.3rem 0.4rem;
    border-radius: var(--radius-sm);
  }

  .name-input:hover,
  .name-input:focus {
    border-color: var(--border);
    background: var(--bg);
    outline: none;
  }

  .type-badge {
    flex-shrink: 0;
    font-size: 0.72rem;
    padding: 0.15rem 0.55rem;
    border-radius: 999px;
    background: var(--bg-elevated-hover);
    color: var(--text-muted);
  }

  .delete-btn {
    flex-shrink: 0;
    border: none;
    background: transparent;
    color: var(--text-faint);
    padding: 0.3rem 0.5rem;
  }

  .delete-btn:hover {
    color: #ff8a8a;
    background: transparent;
  }

  .swatches {
    display: flex;
    gap: 0.55rem;
    flex-wrap: wrap;
  }

  .swatch {
    width: 26px;
    height: 26px;
    border-radius: 50%;
    border: none;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0;
    box-shadow: 0 0 0 2px var(--bg-elevated);
  }

  .swatch.selected {
    box-shadow: 0 0 0 2px var(--bg-elevated), 0 0 0 4px var(--text-muted);
  }

  .language-label {
    margin-top: 1rem;
  }

  .lang-toggle {
    display: flex;
    gap: 0.5rem;
  }

  .lang-btn {
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 0.35rem 0.8rem;
    cursor: pointer;
    background: transparent;
    color: var(--text-muted);
    font-family: inherit;
    font-size: 0.85rem;
  }

  .lang-btn:hover {
    color: var(--text);
  }

  .lang-btn.active {
    background: var(--accent-soft);
    border-color: var(--accent);
    color: var(--text);
    font-weight: 600;
  }

  .enable-toggle {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    cursor: pointer;
    font-size: 0.9rem;
  }

  .enable-toggle input[type='checkbox'] {
    appearance: none;
    width: 34px;
    height: 20px;
    border-radius: 999px;
    background: var(--bg-elevated-hover);
    border: 1px solid var(--border);
    position: relative;
    cursor: pointer;
    flex-shrink: 0;
    transition: background 0.15s ease;
  }

  .enable-toggle input[type='checkbox']::after {
    content: '';
    position: absolute;
    top: 1px;
    left: 1px;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    background: var(--text);
    transition: transform 0.15s ease;
  }

  .enable-toggle input[type='checkbox']:checked {
    background: var(--accent);
    border-color: var(--accent);
  }

  .enable-toggle input[type='checkbox']:checked::after {
    transform: translateX(14px);
  }

  .fields {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    margin-top: 0.9rem;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    font-size: 0.85rem;
    color: var(--text-muted);
  }

  .field input {
    padding: 0.45rem 0.6rem;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--text);
    font-family: inherit;
    font-size: 0.9rem;
  }

  .field input:focus {
    outline: none;
    border-color: var(--accent);
  }

  footer {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    margin-top: 1.1rem;
  }

  button {
    border: none;
    border-radius: var(--radius-sm);
    padding: 0.45rem 0.9rem;
    cursor: pointer;
    background: var(--bg-elevated-hover);
    color: var(--text);
    font-family: inherit;
    font-size: 0.85rem;
  }

  button:hover:not(:disabled) {
    background: var(--border);
  }

  button.primary {
    background: var(--accent);
    color: white;
  }

  button.primary:hover:not(:disabled) {
    background: var(--accent-hover);
  }

  button:disabled {
    opacity: 0.6;
    cursor: default;
  }

  .status {
    font-size: 0.85rem;
  }

  .status.ok {
    color: var(--success);
  }

  .status.error {
    color: #ff8a8a;
  }
</style>
