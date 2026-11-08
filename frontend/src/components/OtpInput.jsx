import React from "react"
import CodeSlots from "./CodeSlots"

export default function OtpInput({
  value = "",
  onChange,
  onComplete,
  length = 6,
  status = "idle",
  disabled = false,
  autoFocus = true,
  accentColor = "#3ecf8e",
  inkColor = "#3ecf8e",
  slotColor,
  digitColor = "#141414",
  dangerColor = "#ef4444",
  slotSize = 46,
  gap = 8,
  radius = 10,
  className = "",
  ...props
}) {
  return (
    <div className={`flex justify-center w-full py-1 ${className}`}>
      <CodeSlots
        length={length}
        value={value}
        onChange={onChange}
        onComplete={onComplete}
        status={status}
        disabled={disabled}
        autoFocus={autoFocus}
        accentColor={accentColor}
        inkColor={inkColor}
        slotColor={slotColor || "#1c1c1c"}
        digitColor={digitColor}
        dangerColor={dangerColor}
        slotSize={slotSize}
        gap={gap}
        radius={radius}
        bounce={0.2}
        settle={0.3}
        rise={8}
        cascade={20}
        {...props}
      />
    </div>
  )
}
