import { onBeforeUnmount, onMounted } from 'vue'

export function usePageSecrets(clear: () => void, restore?: () => void) {
  let controller = new AbortController()
  function end() { controller.abort(); clear() }
  function restored(event: PageTransitionEvent) {
    if (!event.persisted || !controller.signal.aborted) return
    controller = new AbortController()
    restore?.()
  }
  onMounted(() => { window.addEventListener('pagehide', end); window.addEventListener('pageshow', restored) })
  onBeforeUnmount(() => { end(); window.removeEventListener('pagehide', end); window.removeEventListener('pageshow', restored) })
  return { get signal() { return controller.signal } }
}
