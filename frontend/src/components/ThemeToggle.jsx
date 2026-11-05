import React from "react"
import { Sun, Moon } from "lucide-react"
import { useTheme } from "../context/ThemeContext"

export default function ThemeToggle({ showLabel = false, className = "" }) {
  const { theme, toggleTheme, isDark } = useTheme()

  return (
    <button
      type="button"
      onClick={toggleTheme}
      className={`inline-flex items-center justify-center gap-2 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-[#282828] transition-colors p-2 text-xs font-medium focus:outline-none focus-visible:ring-1 focus-visible:ring-[#3ecf8e] cursor-pointer shadow-xs ${className}`}
      title={isDark ? "Switch to Light mode" : "Switch to Dark mode"}
      aria-label={isDark ? "Switch to Light mode" : "Switch to Dark mode"}
    >
      {isDark ? (
        <Sun className="size-4 text-amber-400 transition-transform hover:rotate-45" />
      ) : (
        <Moon className="size-4 text-neutral-700 transition-transform hover:-rotate-12" />
      )}
      {showLabel && (
        <span className="text-[11px] font-medium">
          {isDark ? "Light Mode" : "Dark Mode"}
        </span>
      )}
    </button>
  )
}

