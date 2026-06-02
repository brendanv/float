import * as React from "react"
import { XIcon } from "@phosphor-icons/react"
import { useIsMobile } from "@/hooks/use-mobile"
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from "@/components/ui/drawer"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

const ResponsiveDialogContext = React.createContext(false)

function ResponsiveDialog({ children, ...props }) {
  const isMobile = useIsMobile()
  if (isMobile) {
    return (
      <ResponsiveDialogContext.Provider value={true}>
        <Drawer {...props}>{children}</Drawer>
      </ResponsiveDialogContext.Provider>
    )
  }
  return (
    <ResponsiveDialogContext.Provider value={false}>
      <Dialog {...props}>{children}</Dialog>
    </ResponsiveDialogContext.Provider>
  )
}

function ResponsiveDialogContent({
  children,
  className,
  size,
  showCloseButton = true,
  ...props
}) {
  const isMobile = React.useContext(ResponsiveDialogContext)
  if (isMobile) {
    return (
      <DrawerContent className={cn("overflow-y-auto", className)} {...props}>
        {showCloseButton && (
          <DrawerClose asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              className="absolute top-3 right-3 z-10"
            >
              <XIcon />
              <span className="sr-only">Close</span>
            </Button>
          </DrawerClose>
        )}
        {/* px-4 wrapper provides consistent side padding; header/footer override with px-0 */}
        <div className="px-4">{children}</div>
      </DrawerContent>
    )
  }
  return (
    <DialogContent size={size} showCloseButton={showCloseButton} className={className} {...props}>
      {children}
    </DialogContent>
  )
}

function ResponsiveDialogHeader({ className, ...props }) {
  const isMobile = React.useContext(ResponsiveDialogContext)
  if (isMobile) {
    return <DrawerHeader className={cn("px-0 !text-left", className)} {...props} />
  }
  return <DialogHeader className={className} {...props} />
}

function ResponsiveDialogTitle({ className, ...props }) {
  const isMobile = React.useContext(ResponsiveDialogContext)
  if (isMobile) return <DrawerTitle className={className} {...props} />
  return <DialogTitle className={className} {...props} />
}

function ResponsiveDialogDescription({ className, ...props }) {
  const isMobile = React.useContext(ResponsiveDialogContext)
  if (isMobile) return <DrawerDescription className={className} {...props} />
  return <DialogDescription className={className} {...props} />
}

function ResponsiveDialogFooter({ className, showCloseButton, children, ...props }) {
  const isMobile = React.useContext(ResponsiveDialogContext)
  if (isMobile) {
    return (
      <DrawerFooter className={cn("flex-col-reverse sm:flex-row sm:justify-end px-0", className)} {...props}>
        {showCloseButton && (
          <DrawerClose asChild>
            <Button variant="outline">Close</Button>
          </DrawerClose>
        )}
        {children}
      </DrawerFooter>
    )
  }
  return (
    <DialogFooter showCloseButton={showCloseButton} className={className} {...props}>
      {children}
    </DialogFooter>
  )
}

function ResponsiveDialogClose({ asChild, children, ...props }) {
  const isMobile = React.useContext(ResponsiveDialogContext)
  if (isMobile) return <DrawerClose asChild={asChild} {...props}>{children}</DrawerClose>
  return <DialogClose asChild={asChild} {...props}>{children}</DialogClose>
}

export {
  ResponsiveDialog,
  ResponsiveDialogContent,
  ResponsiveDialogHeader,
  ResponsiveDialogTitle,
  ResponsiveDialogDescription,
  ResponsiveDialogFooter,
  ResponsiveDialogClose,
}
