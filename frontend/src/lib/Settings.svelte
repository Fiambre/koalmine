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

  type Status = { kind: 'idle' | 'testing' | 'ok' | 'error' | 'saving' | 'saved'; message?: string }

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
    max-width: 560px;
    margin: 0 auto;
    padding: 1.5rem;
  }

  h2 {
    margin-top: 0;
  }

  .hint {
    opacity: 0.7;
  }

  .version-line {
    margin: 0;
    font-size: 0.85rem;
    opacity: 0.8;
  }

  .update-banner {
    margin: 0.5rem 0 0;
    display: flex;
    align-items: center;
    gap: 0.75rem;
    font-size: 0.9rem;
  }

  .provider-card {
    background: rgba(255, 255, 255, 0.06);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 8px;
    padding: 1rem;
    margin-bottom: 1rem;
  }

  .enable-toggle {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    cursor: pointer;
  }

  .fields {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    margin-top: 0.75rem;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    font-size: 0.85rem;
  }

  .field input {
    padding: 0.4rem 0.5rem;
    border-radius: 4px;
    border: 1px solid rgba(255, 255, 255, 0.2);
    background: rgba(255, 255, 255, 0.9);
    color: #1b2636;
  }

  footer {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    margin-top: 1rem;
  }

  button {
    border: none;
    border-radius: 4px;
    padding: 0.4rem 0.9rem;
    cursor: pointer;
    background: rgba(255, 255, 255, 0.15);
    color: white;
  }

  button.primary {
    background: #3d7bfd;
  }

  button:disabled {
    opacity: 0.6;
    cursor: default;
  }

  .status {
    font-size: 0.85rem;
  }

  .status.ok {
    color: #7be08f;
  }

  .status.error {
    color: #ff8a8a;
  }
</style>
