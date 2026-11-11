export function wrapDialogFocus(event: KeyboardEvent) {
  if (event.key !== 'Tab') return
  const dialog = event.currentTarget as HTMLDialogElement
  const controls = Array.from(dialog.querySelectorAll<HTMLElement>('a[href], button, input, select, textarea, [tabindex]')).filter(element => {
    const style = getComputedStyle(element)
    return element.tabIndex >= 0 && !element.matches(':disabled, input[type="hidden"]') && !element.closest('[hidden], [inert]') && style.display !== 'none' && style.visibility !== 'hidden'
  })
  const first = controls[0]
  const last = controls.at(-1)
  if (!first || !last) return
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}
