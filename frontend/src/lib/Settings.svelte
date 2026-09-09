<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import {
    ListProviders,
    SaveProviderConfig,
    TestConnection,
    GetAutostartEnabled,
    SetAutostartEnabled,
    GetAppVersion,
    GetUpdateStatus,
    ApplyUpdate,
  } from '../../wailsjs/go/main/App.js'
  import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
  import type { main, updater } from '../../wailsjs/go/models'
  import { ACCENT_COLORS, loadAccent, saveAccent } from './theme'

  type Status = { kind: 'idle' | 'testing' | 'ok' | 'error' | 'saving' | 'saved'; message?: string }

  let selectedAccent = loadAccent()

  function selectAccent(hex: string) {
    selectedAccent = hex
    saveAccent(hex)
  }

  let providerList: main.ProviderInfo[] = []
  let loading = true
  let loadError = ''

  let autostart = false
  let autostartStatus: Status = { kind: 'idle' }

  let appVersion = ''
  let updateInfo: updater.Info | null = null
  let updateStatus: Status = { kind: 'idle' }

  // Per-provider draft form state, keyed by provider name.
  let drafts: Record<string, Record<string, string>> = {}
  let enabledDrafts: Record<string, boolean> = {}
  let statusByProvider: Record<string, Status> = {}

  async function load() {
    loading = true
    loadError = ''
    try {
      providerList = await ListProviders()
      for (const p of providerList) {
        drafts[p.name] = { ...p.values }
        enabledDrafts[p.name] = p.enabled
        statusByProvider[p.name] = { kind: 'idle' }
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
        message: autostart ? 'Se iniciará junto con el sistema.' : 'Ya no se inicia con el sistema.',
      }
    } catch (e) {
      autostart = !autostart
      autostartStatus = { kind: 'error', message: String(e) }
    }
  }

  async function testConnection(p: main.ProviderInfo) {
    statusByProvider[p.name] = { kind: 'testing' }
    try {
      await TestConnection(p.name, drafts[p.name] ?? {})
      statusByProvider[p.name] = { kind: 'ok', message: 'Conexión exitosa.' }
    } catch (e) {
      statusByProvider[p.name] = { kind: 'error', message: String(e) }
    }
  }

  async function save(p: main.ProviderInfo) {
    statusByProvider[p.name] = { kind: 'saving' }
    try {
      await SaveProviderConfig(p.name, enabledDrafts[p.name] ?? false, drafts[p.name] ?? {})
      await load()
      statusByProvider[p.name] = { kind: 'saved', message: 'Guardado.' }
    } catch (e) {
      statusByProvider[p.name] = { kind: 'error', message: String(e) }
    }
  }
</script>

<section class="settings">
  {#if loading}
    <p class="hint">Cargando…</p>
  {:else if loadError}
    <p class="status error">No se pudo cargar la configuración: {loadError}</p>
  {:else}
    <article class="provider-card">
      <h2 class="card-title">Apariencia</h2>
      <p class="hint small">Color de acento</p>
      <div class="swatches">
        {#each ACCENT_COLORS as color (color.value)}
          <button
            class="swatch"
            class:selected={selectedAccent === color.value}
            style="background: {color.value}"
            title={color.name}
            aria-label={color.name}
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
    </article>

    <article class="provider-card">
      <label class="enable-toggle">
        <input type="checkbox" bind:checked={autostart} on:change={toggleAutostart} disabled={autostartStatus.kind === 'saving'} />
        <strong>Iniciar con el sistema</strong>
      </label>
      {#if autostartStatus.kind === 'saved'}
        <span class="status ok">{autostartStatus.message}</span>
      {:else if autostartStatus.kind === 'error'}
        <span class="status error">{autostartStatus.message}</span>
      {/if}
    </article>

    <article class="provider-card">
      <p class="version-line">Versión actual: <strong>{appVersion}</strong></p>
      {#if updateInfo?.available}
        <p class="update-banner">
          Hay una versión nueva disponible: <strong>{updateInfo.version}</strong>
          <button class="primary" on:click={applyUpdate} disabled={updateStatus.kind === 'saving'}>
            {updateStatus.kind === 'saving' ? 'Actualizando…' : 'Actualizar ahora'}
          </button>
        </p>
        {#if updateStatus.kind === 'error'}
          <span class="status error">{updateStatus.message}</span>
        {/if}
      {/if}
    </article>

    <h2>Proveedores</h2>

    {#each providerList as p (p.name)}
      <article class="provider-card">
        <header>
          <label class="enable-toggle">
            <input type="checkbox" bind:checked={enabledDrafts[p.name]} />
            <strong>{p.displayName}</strong>
          </label>
        </header>

        <div class="fields">
          {#each p.fields as field (field.key)}
            <label class="field">
              <span>{field.label}{field.required ? ' *' : ''}</span>
              {#if field.kind === 'secret'}
                <input
                  type="password"
                  autocomplete="off"
                  placeholder={p.secretsSet[field.key] ? '•••••••• (sin cambios)' : field.placeholder}
                  bind:value={drafts[p.name][field.key]}
                />
              {:else}
                <input
                  type={field.kind === 'url' ? 'url' : 'text'}
                  autocomplete="off"
                  placeholder={field.placeholder}
                  bind:value={drafts[p.name][field.key]}
                />
              {/if}
            </label>
          {/each}
        </div>

        <footer>
          <button on:click={() => testConnection(p)} disabled={statusByProvider[p.name]?.kind === 'testing'}>
            {statusByProvider[p.name]?.kind === 'testing' ? 'Probando…' : 'Probar conexión'}
          </button>
          <button class="primary" on:click={() => save(p)} disabled={statusByProvider[p.name]?.kind === 'saving'}>
            {statusByProvider[p.name]?.kind === 'saving' ? 'Guardando…' : 'Guardar'}
          </button>
          {#if statusByProvider[p.name]?.kind === 'ok' || statusByProvider[p.name]?.kind === 'saved'}
            <span class="status ok">{statusByProvider[p.name]?.message}</span>
          {:else if statusByProvider[p.name]?.kind === 'error'}
            <span class="status error">{statusByProvider[p.name]?.message}</span>
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
