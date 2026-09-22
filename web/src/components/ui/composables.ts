import { nextTick, onBeforeUnmount, ref, watch, type Ref } from 'vue'

export function useClickOutside(
  targets: Array<Ref<HTMLElement | null>>,
  handler: (event: MouseEvent) => void,
  enabled?: Ref<boolean>,
) {
  function onPointerDown(event: MouseEvent) {
    if (enabled && !enabled.value) return
    const node = event.target
    if (!(node instanceof Node)) return
    for (const target of targets) {
      if (target.value && target.value.contains(node)) return
    }
    handler(event)
  }
  document.addEventListener('mousedown', onPointerDown)
  onBeforeUnmount(() => document.removeEventListener('mousedown', onPointerDown))
}

export type FloatingAlign = 'start' | 'end'

export function useFloatingPanel(
  open: Ref<boolean>,
  options?: { align?: FloatingAlign; offset?: number; matchWidth?: boolean },
) {
  const triggerRef = ref<HTMLElement | null>(null)
  const panelRef = ref<HTMLElement | null>(null)
  const panelStyle = ref<Record<string, string>>({ top: '0px', left: '0px' })

  async function updatePosition() {
    await nextTick()
    const trigger = triggerRef.value
    const panel = panelRef.value
    if (!trigger || !panel) return
    const rect = trigger.getBoundingClientRect()
    const offset = options?.offset ?? 6
    const panelWidth = panel.offsetWidth
    const panelHeight = panel.offsetHeight
    let top = rect.bottom + offset
    const fitsBelow = top + panelHeight <= window.innerHeight - 8
    const fitsAbove = rect.top - offset - panelHeight >= 8
    if (!fitsBelow && fitsAbove) top = rect.top - offset - panelHeight
    else if (!fitsBelow) top = Math.max(8, window.innerHeight - panelHeight - 8)
    let left = options?.align === 'end' ? rect.right - panelWidth : rect.left
    left = Math.min(Math.max(8, left), Math.max(8, window.innerWidth - panelWidth - 8))
    const style: Record<string, string> = {
      top: `${Math.round(top)}px`,
      left: `${Math.round(left)}px`,
    }
    if (options?.matchWidth) style.minWidth = `${Math.round(rect.width)}px`
    panelStyle.value = style
  }

  function detachListeners() {
    window.removeEventListener('resize', updatePosition)
    window.removeEventListener('scroll', updatePosition, true)
  }

  watch(open, async (isOpen) => {
    if (isOpen) {
      await updatePosition()
      window.addEventListener('resize', updatePosition)
      window.addEventListener('scroll', updatePosition, true)
    } else {
      detachListeners()
    }
  })

  onBeforeUnmount(detachListeners)

  return { triggerRef, panelRef, panelStyle, updatePosition }
}

let scrollLockCount = 0

function applyScrollLock() {
  document.body.style.overflow = scrollLockCount > 0 ? 'hidden' : ''
}

export function useScrollLock(locked: Ref<boolean>) {
  watch(
    locked,
    (isLocked) => {
      scrollLockCount = Math.max(0, scrollLockCount + (isLocked ? 1 : -1))
      applyScrollLock()
    },
    { immediate: true },
  )
  onBeforeUnmount(() => {
    if (locked.value) {
      scrollLockCount = Math.max(0, scrollLockCount - 1)
      applyScrollLock()
    }
  })
}

const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
export function useFocusTrap(container: Ref<HTMLElement | null>, active: Ref<boolean>) {
  let previousFocus: HTMLElement | null = null

  function focusableNodes(): HTMLElement[] {
    const el = container.value
    if (!el) return []
    return Array.from(el.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)).filter(
      (node) => node.getClientRects().length > 0,
    )
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key !== 'Tab') return
    const nodes = focusableNodes()
    const first = nodes[0]
    const last = nodes[nodes.length - 1]
    if (!first || !last) {
      event.preventDefault()
      return
    }
    const current = document.activeElement
    if (event.shiftKey && (current === first || current === container.value)) {
      event.preventDefault()
      last.focus()
    } else if (!event.shiftKey && current === last) {
      event.preventDefault()
      first.focus()
    }
  }

  watch(active, async (isActive) => {
    const el = container.value
    if (!el) return
    if (isActive) {
      previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
      await nextTick()
      const nodes = focusableNodes()
      const target = nodes[0]
      if (target) target.focus()
      else el.focus()
      el.addEventListener('keydown', onKeydown)
    } else {
      el.removeEventListener('keydown', onKeydown)
      if (previousFocus && document.contains(previousFocus)) previousFocus.focus()
      previousFocus = null
    }
  })

  onBeforeUnmount(() => {
    container.value?.removeEventListener('keydown', onKeydown)
    if (active.value && previousFocus && document.contains(previousFocus)) previousFocus.focus()
  })
}

const openDialogStack: symbol[] = []

export function pushDialog(id: symbol) {
  openDialogStack.push(id)
}

export function removeDialog(id: symbol) {
  const index = openDialogStack.indexOf(id)
  if (index >= 0) openDialogStack.splice(index, 1)
}

export function isTopDialog(id: symbol): boolean {
  return openDialogStack[openDialogStack.length - 1] === id
}
