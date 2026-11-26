import React, { useState } from "react"
import { Link } from "react-router-dom"
import { Shield, Palette, Sliders, User, Check } from "lucide-react"
import TopHeader from "../components/TopHeader"
import ThemeToggle from "../components/ThemeToggle"
import { useAuth } from "../context/AuthContext"

const PREF_KEY = "veracity_app_preferences"

function loadPrefs() {
  try {
    const raw = localStorage.getItem(PREF_KEY)
    return raw ? JSON.parse(raw) : { defaultZoom: 14, autoFitBounds: true, metricUnits: true }
  } catch {
    return { defaultZoom: 14, autoFitBounds: true, metricUnits: true }
  }
}

export default function SettingsPage() {
  const { user } = useAuth()
  const [prefs, setPrefs] = useState(loadPrefs)
  const [savedNotice, setSavedNotice] = useState(false)

  const updatePref = (patch) => {
    const next = { ...prefs, ...patch }
    setPrefs(next)
    try {
      localStorage.setItem(PREF_KEY, JSON.stringify(next))
    } catch {}
    setSavedNotice(true)
    setTimeout(() => setSavedNotice(false), 1800)
  }

  return (
    <div className="flex flex-col min-h-screen bg-neutral-50 dark:bg-[#141414]">
      <TopHeader
        title="Settings"
        breadcrumbs={[{ label: "Settings" }]}
        actions={
          savedNotice ? (
            <span className="inline-flex items-center gap-1 text-xs font-medium text-[#24b47e] dark:text-[#3ecf8e]">
              <Check className="size-3.5" /> Saved
            </span>
          ) : null
        }
      />

      <main className="flex-1 w-full max-w-3xl mx-auto px-4 md:px-6 py-6 space-y-5">
        {/* 1. Account */}
        <section className="rounded-lg border border-neutral-200 dark:border-[#2a2a2a] bg-white dark:bg-[#1b1b1b] p-5 space-y-3">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <User className="size-4 text-[#24b47e] dark:text-[#3ecf8e]" />
              <h2 className="text-sm font-semibold text-neutral-900 dark:text-white">Account</h2>
            </div>
            <Link
              to="/profile"
              className="text-xs font-medium text-[#24b47e] dark:text-[#3ecf8e] hover:underline"
            >
              Manage Profile
            </Link>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 text-xs pt-1">
            <div>
              <div className="text-neutral-500 dark:text-neutral-400">Name</div>
              <div className="font-medium text-neutral-900 dark:text-white mt-0.5">
                {user?.name || "—"}
              </div>
            </div>
            <div>
              <div className="text-neutral-500 dark:text-neutral-400">Email</div>
              <div className="font-medium text-neutral-900 dark:text-white mt-0.5">
                {user?.email || "—"}
              </div>
            </div>
          </div>
        </section>

        {/* 2. Appearance */}
        <section className="rounded-lg border border-neutral-200 dark:border-[#2a2a2a] bg-white dark:bg-[#1b1b1b] p-5">
          <div className="flex items-center justify-between gap-4">
            <div className="flex items-center gap-2">
              <Palette className="size-4 text-[#24b47e] dark:text-[#3ecf8e]" />
              <div>
                <h2 className="text-sm font-semibold text-neutral-900 dark:text-white">Appearance</h2>
                <p className="text-xs text-neutral-500 dark:text-neutral-400">
                  Switch between light and dark workspace themes.
                </p>
              </div>
            </div>
            <ThemeToggle />
          </div>
        </section>

        {/* 3. Application Preferences */}
        <section className="rounded-lg border border-neutral-200 dark:border-[#2a2a2a] bg-white dark:bg-[#1b1b1b] p-5 space-y-4">
          <div className="flex items-center gap-2">
            <Sliders className="size-4 text-[#24b47e] dark:text-[#3ecf8e]" />
            <h2 className="text-sm font-semibold text-neutral-900 dark:text-white">
              Application Preferences
            </h2>
          </div>

          <label className="flex items-center justify-between gap-4 text-xs cursor-pointer">
            <div>
              <div className="font-medium text-neutral-900 dark:text-white">Auto-fit map bounds on load</div>
              <div className="text-neutral-500 dark:text-neutral-400">
                Automatically zoom to imported rasters and vector layers.
              </div>
            </div>
            <input
              type="checkbox"
              checked={Boolean(prefs.autoFitBounds)}
              onChange={(e) => updatePref({ autoFitBounds: e.target.checked })}
              className="accent-[#3ecf8e] size-4"
            />
          </label>

          <label className="flex items-center justify-between gap-4 text-xs cursor-pointer">
            <div>
              <div className="font-medium text-neutral-900 dark:text-white">Metric measurement units</div>
              <div className="text-neutral-500 dark:text-neutral-400">
                Display distances in meters/kilometers and area in m²/hectares.
              </div>
            </div>
            <input
              type="checkbox"
              checked={Boolean(prefs.metricUnits)}
              onChange={(e) => updatePref({ metricUnits: e.target.checked })}
              className="accent-[#3ecf8e] size-4"
            />
          </label>
        </section>

        {/* 4. Security */}
        <section className="rounded-lg border border-neutral-200 dark:border-[#2a2a2a] bg-white dark:bg-[#1b1b1b] p-5 space-y-3">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Shield className="size-4 text-[#24b47e] dark:text-[#3ecf8e]" />
              <div>
                <h2 className="text-sm font-semibold text-neutral-900 dark:text-white">Security</h2>
                <p className="text-xs text-neutral-500 dark:text-neutral-400">
                  Update your password, email verification, and active sessions.
                </p>
              </div>
            </div>
            <Link
              to="/profile"
              className="px-3 py-1.5 rounded-[6px] border border-neutral-200 dark:border-[#323232] text-xs font-medium text-neutral-700 dark:text-neutral-200 hover:border-[#3ecf8e] transition-colors"
            >
              Security Settings
            </Link>
          </div>
        </section>
      </main>
    </div>
  )
}
