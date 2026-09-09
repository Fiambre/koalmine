import './style.css'
import { mount } from 'svelte'
import App from './App.svelte'
import { applyAccent, loadAccent } from './lib/theme'

applyAccent(loadAccent())

const app = mount(App, {
  target: document.getElementById('app')!
})

export default app
