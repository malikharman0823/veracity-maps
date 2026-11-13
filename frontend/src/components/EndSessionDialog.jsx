import React, { useState } from "react"
import { Trash2, AlertTriangle, Loader2 } from "lucide-react"
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { endSession, clearActiveSessionId, createSession } from "../api"
import HoldButton from "./HoldButton"

export default function EndSessionDialog({ trigger, onEnded, className = "" }) {
  const [open, setOpen] = useState(false)
  const [ending, setEnding] = useState(false)

  async function handleConfirmEnd() {
    setEnding(true)
    try {
      await endSession()
    } catch (err) {
      console.warn("Clean session termination encountered an issue:", err)
    } finally {
      clearActiveSessionId()
      try {
        await createSession()
      } catch (err) {
        console.warn("Failed to create new session after termination:", err)
      }
      setEnding(false)
      setOpen(false)
      if (onEnded) {
        onEnded()
      } else {
        window.location.href = "/"
      }
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={setOpen}>
      <AlertDialogTrigger
        render={
          trigger || (
            <Button
              variant="outline"
              size="sm"
              className={`h-8 border-red-500/40 bg-white dark:bg-[#202020] text-red-500 hover:text-red-400 hover:border-red-500 hover:bg-red-500/10 text-xs font-normal gap-1.5 transition-colors rounded-[6px] cursor-pointer ${className}`}
              title="End session and clear workspace data"
            >
              <Trash2 className="size-3.5 text-red-500" />
              <span className="hidden sm:inline">End Session</span>
            </Button>
          )
        }
      />
      <AlertDialogContent className="max-w-md bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] shadow-xl p-5 rounded-[12px] text-neutral-900 dark:text-white">
        <AlertDialogHeader className="space-y-2">
          <div className="flex items-center gap-3 text-red-600 dark:text-red-400">
            <div className="flex h-10 w-10 items-center justify-center rounded-full bg-red-100/80 dark:bg-red-950/40">
              <AlertTriangle className="size-5 text-red-600 dark:text-red-400" />
            </div>
            <AlertDialogTitle className="text-base font-semibold text-neutral-900 dark:text-white">
              End Session & Clear Workspace?
            </AlertDialogTitle>
          </div>
          <AlertDialogDescription className="text-xs text-neutral-500 dark:text-neutral-400 leading-relaxed pt-1">
            This workspace uses temporary session storage. Ending the session will immediately
            and permanently remove all uploaded datasets, raster imagery, georeferencing
            ground control points, layer configurations, and pending exports.
            <br />
            <br />
            A new empty workspace session will be started. This action cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter className="mt-4 flex sm:flex-row items-center justify-end gap-2.5 border-t border-neutral-200 dark:border-[#2e2e2e] pt-4">
          <AlertDialogCancel
            disabled={ending}
            className="h-8 rounded-[6px] text-xs font-medium border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#282828] cursor-pointer"
          >
            Keep Session
          </AlertDialogCancel>

          <HoldButton
            doneLabel="Session Ended"
            backgroundColor="#27272a"
            fillColor="#05f800"
            textColor="#f5f5f5"
            fillTextColor="#000000"
            size="sm"
            radius={14}
            fillDirection="right"
            holdTime={2000}
            releaseTime={200}
            pressScale={0.97}
            wave
            waveAmplitude={6}
            glow
            resetAfter={1200}
            disabled={ending}
            onHold={handleConfirmEnd}
            icon={ending ? <Loader2 className="size-3.5 animate-spin" /> : <Trash2 className="size-3.5" />}
          >
            {ending ? "Clearing…" : "Hold to End Session"}
          </HoldButton>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
