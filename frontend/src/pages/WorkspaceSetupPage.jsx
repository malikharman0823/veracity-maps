import React, { useEffect, useState } from "react"
import { useNavigate, useParams } from "react-router-dom"
import { Layers, ArrowLeft } from "lucide-react"
import { getMap, createMap } from "../api"
import ThemeToggle from "../components/ThemeToggle"
import UnifiedDatasetUploadPanel from "../components/gis/UnifiedDatasetUploadPanel"

export default function WorkspaceSetupPage() {
  const { mapId } = useParams()
  const navigate = useNavigate()
  const [workspaceId, setWorkspaceId] = useState(mapId || "")
  const [workspaceName, setWorkspaceName] = useState("New Workspace")
  const [loading, setLoading] = useState(Boolean(mapId))

  useEffect(() => {
    let active = true
    if (mapId) {
      setWorkspaceId(mapId)
      setLoading(true)
      getMap(mapId)
        .then((res) => {
          if (!active) return
          const m = res?.map || res
          if (m?.name) setWorkspaceName(m.name)
        })
        .catch(() => {})
        .finally(() => {
          if (active) setLoading(false)
        })
    } else {
      setLoading(false)
    }
    return () => {
      active = false
    }
  }, [mapId])

  const handleComplete = async (result) => {
    let targetId = workspaceId
    if (!targetId) {
      try {
        const created = await createMap(
          workspaceName || `Workspace ${new Date().toLocaleDateString()}`,
          "Created via Unified Upload Page"
        )
        targetId = created?.id
      } catch {
        navigate("/dashboard")
        return
      }
    }
    if (targetId) {
      navigate(`/maps/${encodeURIComponent(targetId)}/edit?baseLayerConfirmed=1`)
    } else {
      navigate("/dashboard")
    }
  }

  return (
    <div className="min-h-screen bg-[#fafafa] dark:bg-[#14161a] text-neutral-900 dark:text-neutral-100">
      <header className="border-b border-neutral-200 dark:border-[#24272f] bg-white/80 dark:bg-[#191b1f]/80 backdrop-blur-md sticky top-0 z-30">
        <div className="max-w-5xl mx-auto px-6 h-14 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <button
              type="button"
              onClick={() => navigate("/dashboard")}
              className="p-1.5 rounded-[6px] text-neutral-500 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#2a2d34] cursor-pointer"
              title="Back to Dashboard"
            >
              <ArrowLeft className="w-4 h-4" />
            </button>
            <div className="w-8 h-8 rounded-[8px] bg-[#3ecf8e]/15 text-[#3ecf8e] flex items-center justify-center">
              <Layers className="w-4 h-4" />
            </div>
            <div>
              <span className="font-bold text-sm text-neutral-900 dark:text-white">
                Set Up Your Workspace
              </span>
              {workspaceName && (
                <span className="ml-2 text-xs text-neutral-500 dark:text-neutral-400">
                  / {workspaceName}
                </span>
              )}
            </div>
          </div>
          <ThemeToggle />
        </div>
      </header>

      <main className="max-w-5xl mx-auto px-6 py-7 space-y-5">
        <div>
          <h1 className="text-xl font-bold tracking-tight text-neutral-900 dark:text-white">
            Set Up Your Workspace
          </h1>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-1">
            Add geographic datasets now or start with the default World Map and add them later. All uploads are optional.
          </p>
        </div>

        {loading ? (
          <div className="py-16 text-center text-xs text-neutral-500">
            Loading workspace setup…
          </div>
        ) : (
          <UnifiedDatasetUploadPanel
            workspaceId={workspaceId}
            workspaceName={workspaceName}
            primaryActionLabel="Open Workspace"
            showBackButton
            onBack={() => navigate("/dashboard")}
            onComplete={handleComplete}
          />
        )}
      </main>
    </div>
  )
}
