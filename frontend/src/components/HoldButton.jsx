import React, { useState, useRef, useEffect, useCallback } from "react"
import "./HoldButton.css"

export default function HoldButton({
  children = "Hold to delete",
  doneLabel = "Deleted",
  backgroundColor = "#1e1e22",
  fillColor = "#ef4444",
  textColor = "#f87171",
  fillTextColor = "#ffffff",
  size = "md",
  radius = 10,
  fillDirection = "right",
  holdTime = 1000,
  releaseTime = 200,
  pressScale = 0.96,
  wave = true,
  waveAmplitude = 6,
  glow = true,
  resetAfter = 1200,
  onHold,
  disabled = false,
  className = "",
  style = {},
  title,
  type = "button",
  icon,
}) {
  const [progress, setProgress] = useState(0) // 0 to 1
  const [status, setStatus] = useState("idle") // "idle" | "holding" | "done"
  const buttonRef = useRef(null)
  const animFrameRef = useRef(null)
  const startTimeRef = useRef(null)
  const startProgressRef = useRef(0)
  const isHeldRef = useRef(false)
  const resetTimerRef = useRef(null)
  const executeTimerRef = useRef(null)
  const [waveOffset, setWaveOffset] = useState(0)

  // Clean up timers & animation frames on unmount
  useEffect(() => {
    return () => {
      if (animFrameRef.current) cancelAnimationFrame(animFrameRef.current)
      if (resetTimerRef.current) clearTimeout(resetTimerRef.current)
      if (executeTimerRef.current) clearTimeout(executeTimerRef.current)
    }
  }, [])

  const finishHold = useCallback(() => {
    setStatus("done")
    setProgress(1)
    isHeldRef.current = false

    if (navigator.vibrate) {
      try {
        navigator.vibrate([30, 40, 50])
      } catch {
        // Ignore haptics errors
      }
    }

    // Give the user 450ms to see and feel the "Deleted" state and pop animation
    // before executing the onHold callback which might unmount this component
    if (onHold) {
      executeTimerRef.current = setTimeout(() => {
        try {
          onHold()
        } catch (err) {
          console.error("HoldButton onHold error:", err)
        }
      }, 450)
    }

    if (resetAfter > 0) {
      resetTimerRef.current = setTimeout(() => {
        setStatus("idle")
        setProgress(0)
      }, resetAfter + 450)
    }
  }, [onHold, resetAfter])

  const startHolding = useCallback(
    (e) => {
      if (disabled || status === "done") return
      if (e && e.button !== undefined && e.button !== 0) return // Left click only

      // Capture pointer so moving cursor slightly doesn't abort
      if (e && e.currentTarget && e.currentTarget.setPointerCapture && e.pointerId !== undefined) {
        try {
          e.currentTarget.setPointerCapture(e.pointerId)
        } catch {
          // Pointer capture may fail on certain synthetic events
        }
      }

      if (resetTimerRef.current) {
        clearTimeout(resetTimerRef.current)
        resetTimerRef.current = null
      }
      if (executeTimerRef.current) {
        clearTimeout(executeTimerRef.current)
        executeTimerRef.current = null
      }

      isHeldRef.current = true
      setStatus("holding")
      startTimeRef.current = performance.now()
      startProgressRef.current = progress

      if (animFrameRef.current) {
        cancelAnimationFrame(animFrameRef.current)
      }

      const animateHold = (now) => {
        if (!isHeldRef.current) return

        const elapsed = now - startTimeRef.current
        const remainingProgress = 1 - startProgressRef.current
        const timeNeeded = remainingProgress * holdTime

        let nextProgress =
          timeNeeded > 0
            ? startProgressRef.current + elapsed / holdTime
            : 1

        // Animate wave crest offset continuously
        setWaveOffset((prev) => (prev + 3) % 40)

        if (nextProgress >= 1) {
          finishHold()
        } else {
          setProgress(Math.min(1, Math.max(0, nextProgress)))
          animFrameRef.current = requestAnimationFrame(animateHold)
        }
      }

      animFrameRef.current = requestAnimationFrame(animateHold)
    },
    [disabled, status, progress, holdTime, finishHold]
  )

  const stopHolding = useCallback(
    (e) => {
      if (!isHeldRef.current || status === "done") return

      // Release pointer capture
      if (e && e.currentTarget && e.currentTarget.releasePointerCapture && e.pointerId !== undefined) {
        try {
          e.currentTarget.releasePointerCapture(e.pointerId)
        } catch {
          // Ignore release errors
        }
      }

      isHeldRef.current = false
      setStatus("idle")

      if (animFrameRef.current) {
        cancelAnimationFrame(animFrameRef.current)
      }

      const releaseStart = performance.now()
      const releaseProgressStart = progress

      const animateRelease = (now) => {
        const elapsed = now - releaseStart
        const frac = Math.min(1, elapsed / releaseTime)
        const current = releaseProgressStart * (1 - frac)

        setProgress(Math.max(0, current))

        if (frac < 1 && !isHeldRef.current) {
          animFrameRef.current = requestAnimationFrame(animateRelease)
        } else {
          setProgress(0)
        }
      }

      animFrameRef.current = requestAnimationFrame(animateRelease)
    },
    [status, progress, releaseTime]
  )

  // Keyboard handlers
  const handleKeyDown = (e) => {
    if (e.key === " " || e.key === "Enter") {
      e.preventDefault()
      if (!isHeldRef.current) {
        startHolding(e)
      }
    }
  }

  const handleKeyUp = (e) => {
    if (e.key === " " || e.key === "Enter") {
      e.preventDefault()
      stopHolding(e)
    }
  }

  const isHolding = status === "holding"
  const isDone = status === "done"

  const scale = isHolding ? pressScale : 1
  const pct = (progress * 100).toFixed(2)

  // Build clip-path based on fillDirection
  const getClipPath = () => {
    switch (fillDirection) {
      case "left":
        return `inset(0 0 0 ${100 - parseFloat(pct)}%)`
      case "up":
        return `inset(${100 - parseFloat(pct)}% 0 0 0)`
      case "down":
        return `inset(0 0 ${100 - parseFloat(pct)}% 0)`
      case "right":
      default:
        return `inset(0 ${100 - parseFloat(pct)}% 0 0)`
    }
  }

  const glowStyle =
    glow && (isHolding || isDone)
      ? {
          boxShadow: `0 0 ${isDone ? 22 : 14}px ${fillColor}77, inset 0 0 10px ${fillColor}33`,
        }
      : {}

  const currentLabel = isDone ? doneLabel : children

  return (
    <button
      ref={buttonRef}
      type={type}
      disabled={disabled}
      title={title}
      aria-label={typeof currentLabel === "string" ? currentLabel : "Hold to Delete"}
      onPointerDown={startHolding}
      onPointerUp={stopHolding}
      onPointerLeave={stopHolding}
      onPointerCancel={stopHolding}
      onKeyDown={handleKeyDown}
      onKeyUp={handleKeyUp}
      onContextMenu={(e) => e.preventDefault()}
      className={`hold-button hold-button--${size} ${
        isDone ? "hold-button--done" : ""
      } ${isHolding ? "hold-button--holding" : ""} ${className}`}
      style={{
        backgroundColor,
        borderRadius: `${radius}px`,
        transform: `scale(${scale})`,
        ...glowStyle,
        ...style,
      }}
    >
      {/* 1. Base Layer (underneath) */}
      <span
        className="hold-button__layer hold-button__layer--base"
        style={{
          color: textColor,
        }}
      >
        {icon && <span className="hold-button__icon">{icon}</span>}
        <span className="hold-button__text">{currentLabel}</span>
      </span>

      {/* 2. Fill Layer (pixel-perfect overlay revealed via clip-path) */}
      <span
        className="hold-button__layer hold-button__layer--fill"
        style={{
          backgroundColor: fillColor,
          color: fillTextColor,
          clipPath: getClipPath(),
          WebkitClipPath: getClipPath(),
        }}
        aria-hidden="true"
      >
        {icon && <span className="hold-button__icon">{icon}</span>}
        <span className="hold-button__text">{currentLabel}</span>

        {/* 3. Wave Crest along leading progress edge */}
        {wave && progress > 0.01 && progress < 0.99 && (
          <div
            className="hold-button__wave-crest"
            style={{
              left: `${pct}%`,
            }}
          >
            <svg
              className="hold-button__wave-svg"
              viewBox="0 0 16 80"
              preserveAspectRatio="none"
              style={{
                transform: `translateY(${waveOffset - 20}px)`,
                fill: fillColor,
              }}
            >
              <path
                d={`M 0,0 Q ${waveAmplitude},10 0,20 T 0,40 T 0,60 T 0,80 T 0,100 T 0,120 V 120 H 16 V 0 Z`}
              />
            </svg>
          </div>
        )}
      </span>

      {/* 4. Completion ripple checkmark / pulse */}
      {isDone && (
        <span className="hold-button__done-badge" style={{ color: fillTextColor }}>
          ✓
        </span>
      )}
    </button>
  )
}
