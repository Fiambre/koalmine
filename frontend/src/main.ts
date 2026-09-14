import './style.css'
import { mount } from 'svelte'
import { waitLocale } from 'svelte-i18n'
import './lib/i18n'
import App from './App.svelte'
import { applyAccent, loadAccent } from './lib/theme'

applyAccent(loadAccent())

// Wait for the initial locale's messages to load so the first render
// already has translated text instead of flashing raw message keys.
waitLocale().then(() => {
  mount(App, {
    target: document.getElementById('app')!,
  })
})
