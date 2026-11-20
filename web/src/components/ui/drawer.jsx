import * as React from "react"
import { Drawer as DrawerPrimitive } from "vaul"

import { cn } from "@/lib/utils"

// repositionInputs is off: vaul's version pins the drawer to a fixed pixel
// height/bottom when an input is focused and guesses the keyboard state by
// toggling a flag on each >60px viewport change. iOS fires several resize
// events while the keyboard animates, so the flag desyncs and the drawer is
// left shrunk/offset (content scrolled away) after the keyboard closes.
// DrawerContent follows the visual viewport itself instead (see
// useKeyboardInset).
function Drawer({
  ...props
}) {
  return <DrawerPrimitive.Root data-slot="drawer" repositionInputs={false} {...props} />;
}

function DrawerTrigger({
  ...props
}) {
  return <DrawerPrimitive.Trigger data-slot="drawer-trigger" {...props} />;
}

function DrawerPortal({
  ...props
}) {
  return <DrawerPrimitive.Portal data-slot="drawer-portal" {...props} />;
}

function DrawerClose({
  ...props
}) {
  return <DrawerPrimitive.Close data-slot="drawer-close" {...props} />;
}

// forwardRef: vaul's Radix Portal/Presence attaches a ref to this element.
const DrawerOverlay = React.forwardRef(function DrawerOverlay({
  className,
  ...props
}, ref) {
  return (
    <DrawerPrimitive.Overlay
      ref={ref}
      data-slot="drawer-overlay"
      className={cn(
        "fixed inset-0 z-50 bg-black/10 supports-backdrop-filter:backdrop-blur-xs data-open:animate-in data-open:fade-in-0 data-closed:animate-out data-closed:fade-out-0",
        className
      )}
      {...props} />
  );
});

// Gap kept above a drawer that the on-screen keyboard has squeezed.
const KEYBOARD_TOP_GAP = 24

// useKeyboardInset reports how much of the layout viewport the on-screen
// keyboard covers, and the visible height above it. It is recomputed from
// window.visualViewport on every event rather than tracked as open/closed
// state, so it cannot get stuck. Android Chrome resizes the layout viewport
// itself (interactive-widget=resizes-content in index.html), so the inset is
// 0 there; iOS Safari only shrinks the visual viewport.
function useKeyboardInset() {
  const [state, setState] = React.useState({ inset: 0, height: 0 })
  React.useEffect(() => {
    const vv = window.visualViewport
    if (!vv) return
    function update() {
      const inset = Math.max(0, Math.round(window.innerHeight - vv.height - vv.offsetTop))
      const height = Math.round(vv.height)
      setState((prev) =>
        prev.inset === inset && prev.height === height ? prev : { inset, height }
      )
    }
    update()
    vv.addEventListener("resize", update)
    vv.addEventListener("scroll", update)
    return () => {
      vv.removeEventListener("resize", update)
      vv.removeEventListener("scroll", update)
    }
  }, [])
  return state
}

function DrawerContent({
  className,
  children,
  style,
  ...props
}) {
  const { inset, height } = useKeyboardInset()
  // Only bottom/max-height move: the drawer keeps sizing to its content, so it
  // grows/shrinks naturally as fields are added and returns to its normal
  // place as soon as the keyboard closes.
  const keyboardStyle = inset > 0
    ? { bottom: inset, maxHeight: Math.max(0, height - KEYBOARD_TOP_GAP) }
    : null
  // Once the drawer has shrunk above the keyboard, keep the focused field in
  // its scroll area visible. Popovers position themselves, so skip those.
  React.useEffect(() => {
    if (inset === 0) return
    const el = document.activeElement
    if (!el?.closest?.("[data-vaul-drawer]") || el.closest("[data-slot=popover-content]")) return
    const id = requestAnimationFrame(() => el.scrollIntoView({ block: "nearest" }))
    return () => cancelAnimationFrame(id)
  }, [inset, height])
  return (
    <DrawerPortal data-slot="drawer-portal">
      <DrawerOverlay />
      <DrawerPrimitive.Content
        data-slot="drawer-content"
        className={cn(
          "group/drawer-content fixed z-50 flex h-auto flex-col bg-popover text-body text-popover-foreground data-[vaul-drawer-direction=bottom]:inset-x-0 data-[vaul-drawer-direction=bottom]:bottom-0 data-[vaul-drawer-direction=bottom]:mt-24 data-[vaul-drawer-direction=bottom]:max-h-[80vh] data-[vaul-drawer-direction=bottom]:rounded-t-xl data-[vaul-drawer-direction=bottom]:border-t data-[vaul-drawer-direction=left]:inset-y-0 data-[vaul-drawer-direction=left]:left-0 data-[vaul-drawer-direction=left]:w-3/4 data-[vaul-drawer-direction=left]:rounded-none data-[vaul-drawer-direction=left]:border-r data-[vaul-drawer-direction=right]:inset-y-0 data-[vaul-drawer-direction=right]:right-0 data-[vaul-drawer-direction=right]:w-3/4 data-[vaul-drawer-direction=right]:rounded-none data-[vaul-drawer-direction=right]:border-l data-[vaul-drawer-direction=top]:inset-x-0 data-[vaul-drawer-direction=top]:top-0 data-[vaul-drawer-direction=top]:mb-24 data-[vaul-drawer-direction=top]:max-h-[80vh] data-[vaul-drawer-direction=top]:rounded-b-xl data-[vaul-drawer-direction=top]:border-b data-[vaul-drawer-direction=left]:sm:max-w-sm data-[vaul-drawer-direction=right]:sm:max-w-sm",
          className
        )}
        style={keyboardStyle ? { ...style, ...keyboardStyle } : style}
        {...props}>
        <div
          className="mx-auto mt-4 hidden h-1 w-[100px] shrink-0 rounded-full bg-muted group-data-[vaul-drawer-direction=bottom]/drawer-content:block" />
        {children}
      </DrawerPrimitive.Content>
    </DrawerPortal>
  );
}

function DrawerHeader({
  className,
  ...props
}) {
  return (
    <div
      data-slot="drawer-header"
      className={cn(
        "flex flex-col gap-0.5 p-4 text-left",
        className
      )}
      {...props} />
  );
}

function DrawerFooter({
  className,
  ...props
}) {
  return (
    <div
      data-slot="drawer-footer"
      className={cn("mt-auto flex flex-col gap-2 p-4", className)}
      {...props} />
  );
}

function DrawerTitle({
  className,
  ...props
}) {
  return (
    <DrawerPrimitive.Title
      data-slot="drawer-title"
      className={cn("font-heading text-heading font-semibold text-foreground", className)}
      {...props} />
  );
}

function DrawerDescription({
  className,
  ...props
}) {
  return (
    <DrawerPrimitive.Description
      data-slot="drawer-description"
      className={cn("text-control text-muted-foreground", className)}
      {...props} />
  );
}

export {
  Drawer,
  DrawerPortal,
  DrawerOverlay,
  DrawerTrigger,
  DrawerClose,
  DrawerContent,
  DrawerHeader,
  DrawerFooter,
  DrawerTitle,
  DrawerDescription,
}
