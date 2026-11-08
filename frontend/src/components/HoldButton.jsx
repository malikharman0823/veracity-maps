import React, { useState, useRef, useEffect, useCallback } from "react"
import "./HoldButton.css"

export default function HoldButton({
  children = "Hold to confirm",
  doneLabel = "Done",
  backgroundColor = "#27272a",
  fillColor = "#05f800",
  textColor = "#f5f5f5",
  fillTextColor = "#000000",
  size = "md",
  radius = 14,
  fillDirection = "right",
  holdTime = 2000,
  releaseTime = 200,
  pressScale = 0.97,
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

  // Clean up any ongoing frame or timer on unmount
  useEffect(() => {
    return () => {
      if (animFrameRef.current) cancelAnimationFrame(animFrameRef.current)
      if (resetTimerRef.current) clearTimeout(resetTimerRef.current)
    }
  }, [])

  const finishHold = useCallback(() => {
    setStatus("done")
    setProgress(1)
    isHeldRef.current = false

    if (navigator.vibrate) {
      try {
        navigator.vibrate(40)
      } catch {
        // Ignore haptics errors
      }
    }

    if (onHold) {
      onHold()
    }

    if (resetAfter > 0) {
      resetTimerRef.current = setTimeout(() => {
        setStatus("idle")
        setProgress(0)
      }, resetAfter)
    }
  }, [onHold, resetAfter])

  const startHolding = useCallback(
    (e) => {
      if (disabled || status === "done") return
      if (e && e.button !== undefined && e.button !== 0) return // Left click only

      if (resetTimerRef.current) {
        clearTimeout(resetTimerRef.current)
        resetTimerRef.current = null
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
            ? startProgressRef.current + (elapsed / holdTime)
            : 1

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

  const stopHolding = useCallback(() => {
    if (!isHeldRef.current || status === "done") return

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
  }, [status, progress, releaseTime])

  // Keyboard handlers
  const handleKeyDown = (e) => {
    if (e.key === " " || e.key === "Enter") {
      e.preventDefault()
      if (!isHeldRef.current) {
        startHolding()
      }
    }
  }

  const handleKeyUp = (e) => {
    if (e.key === " " || e.key === "Enter") {
      e.preventDefault()
      stopHolding()
    }
  }

  // Calculate fill styles based on direction
  const getFillStyle = () => {
    const pct = `${(progress * 100).toFixed(2)}%`
    switch (fillDirection) {
      case "left":
        return { right: 0, width: pct, top: 0, bottom: 0 }
      case "up":
        return { bottom: 0, height: pct, left: 0, right: 0 }
      case "down":
        return { top: 0, height: pct, left: 0, right: 0 }
      case "right":
      default:
        return { left: 0, width: pct, top: 0, bottom: 0 }
    }
  }

  // Wave path generator
  const getWavePath = () => {
    const amp = waveAmplitude
    const w = 12
    return `M 0,0 Q ${w},${amp} 0,${amp * 2} T 0,${amp * 4} T 0,${amp * 6} T 0,${amp * 8} V 200 H 0 Z`
  }

  const isHolding = status === "holding"
  const isDone = status === "done"

  const scale = isHolding ? pressScale : 1
  const glowStyle =
    glow && (isHolding || isDone)
      ? {
          boxShadow: `0 0 ${isDone ? 24 : 16}px ${fillColor}66, inset 0 0 12px ${fillColor}33`,
        }
      : {}

  const currentLabel = isDone ? doneLabel : children

  return (
    <button
      ref={buttonRef}
      type={type}
      disabled={disabled}
      title={title}
      aria-label={typeof currentLabel === "string" ? currentLabel : "Hold Button"}
      onPointerDown={startHolding}
      onPointerUp={stopHolding}
      onPointerLeave={stopHolding}
      onPointerCancel={stopHolding}
      onKeyDown={handleKeyDown}
      onKeyUp={handleKeyUp}
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
      {/* Base text layer */}
      <span
        className="hold-button__label-base"
        style={{
          color: textColor,
        }}
      >
        {icon}
        {currentLabel}
      </span>

      {/* Progress fill layer */}
      <div
        className="hold-button__fill-container"
        style={{
          ...getFillStyle(),
          borderRadius: `${radius}px`,
        }}
      >
        <div
          className="hold-button__fill-bg"
          style={{
            backgroundColor: fillColor,
          }}
        />

        {/* Wave crest */}
        {wave && progress > 0.02 && progress < 0.98 && (
          <div
            className="hold-button__wave"
            style={{
              right: fillDirection === "right" ? "-6px" : undefined,
              left: fillDirection === "left" ? "-6px" : undefined,
            }}
          >
            <svg
              className="hold-button__wave-svg"
              viewBox="0 0 20 80"
              preserveAspectRatio="none"
              style={{
                fill: fillColor,
                opacity: 0.85,
              }}
            >
              <path d={getWavePath()} />
            </svg>
          </div>
        )}

        {/* Overlaid text layer with fillTextColor */}
        <span
          className="hold-button__label-fill"
          style={{
            color: fillTextColor,
            width: buttonRef.current ? `${buttonRef.current.offsetWidth}px` : "100%",
          }}
        >
          {icon}
          {currentLabel}
        </span>
      </div>
    </button>
  )
}

