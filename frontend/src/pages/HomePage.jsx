import React, { useEffect, useState } from "react"
import { Link, useNavigate } from "react-router-dom"
import { Plus, Clock, FolderOpen, ArrowRight, Loader2, X, Layers } from "lucide-react"
import TopHeader from "../components/TopHeader"
import UnifiedDatasetUploadPanel from "../components/gis/UnifiedDatasetUploadPanel"
import { listMaps, createMap } from "../api"

function formatDate(val) {
  if (!val) return "—"
  const d = new Date(val)
  if (isNaN(d.getTime()) || d.getFullYear() <= 1) return "—"
  return d.toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" })
}

function resolveProjectId(item) {
  return item?.map_id || item?.id || ""
}

function resolveProjectName(item, byIdMap) {
  const id = resolveProjectId(item)
  const matched = id ? byIdMap.get(id) : null
  const raw = item?.name || item?.map_name || item?.title || matched?.name || matched?.title || ""
  return String(raw).trim() || "Untitled Project"
}

export default function HomePage() {
  const navigate = useNavigate()

  const [myMaps, setMyMaps] = useState([])
  const [recentMaps, setRecentMaps] = useState([])
  const [loadingProjects, setLoadingProjects] = useState(true)
  const [projectError, setProjectError] = useState("")

  const [showCreate, setShowCreate] = useState(false)
  const [newName, setNewName] = useState("")
  const [creating, setCreating] = useState(false)

  const fetchProjects = async () => {
    setLoadingProjects(true)
    setProjectError("")
    try {
      const data = await listMaps()
      const my = data?.my_maps || []
      const recent = data?.recent_maps || []
      setMyMaps(my)
      setRecentMaps(recent)
      window.dispatchEvent(
        new CustomEvent("veracity:maps-updated", { detail: { myMaps: my, sharedWithMe: [] } })
      )
    } catch {
      setProjectError("Unable to load your recent projects.")
    } finally {
      setLoadingProjects(false)
    }
  }

  useEffect(() => {
    fetchProjects()
  }, [])

  const handleCreateProject = async (e) => {
    e.preventDefault()
    if (!newName.trim() || creating) return
    setCreating(true)
    try {
      const res = await createMap(newName.trim(), "")
      const created = res?.map || res
      setNewName("")
      setShowCreate(false)
      await fetchProjects()
      if (created?.id) navigate(`/maps/${encodeURIComponent(created.id)}/edit`)
    } catch {
      setProjectError("Failed to create project.")
    } finally {
      setCreating(false)
    }
  }

  const resolveOrCreateProjectId = async (suggestedName = "") => {
    if (myMaps.length > 0 && myMaps[0]?.id) {
      return myMaps[0].id
    }
    try {
      const clean = String(suggestedName || "").trim() || `Workspace ${new Date().toLocaleDateString()}`
      const res = await createMap(clean, "")
      const created = res?.map || res
      if (created?.id) {
        setMyMaps((prev) => [created, ...prev])
        return created.id
      }
    } catch {}
    return "default"
  }

  const handleUploadComplete = async (result) => {
    const targetId = result?.projectId || (myMaps.length > 0 ? myMaps[0]?.id : "")
    if (targetId && targetId !== "default") {
      navigate(`/maps/${encodeURIComponent(targetId)}/edit`)
      return
    }
    const createdId = await resolveOrCreateProjectId()
    if (createdId && createdId !== "default") {
      navigate(`/maps/${encodeURIComponent(createdId)}/edit`)
      return
    }
    navigate("/projects")
  }

  // Build lookup map so recent_maps entries always resolve their real project name
  const byIdMap = new Map()
  for (const m of myMaps) {
    if (m?.id) byIdMap.set(m.id, m)
  }

  // Deduplicate recent projects by map_id / id, falling back cleanly to myMaps
  const seen = new Set()
  const recentList = []
  for (const item of [...recentMaps, ...myMaps]) {
    const pid = resolveProjectId(item)
    if (!pid || seen.has(pid)) continue
    seen.add(pid)
    recentList.push({
      id: pid,
      name: resolveProjectName(item, byIdMap),
      date: item.updated_at || item.last_opened_at || item.created_at || byIdMap.get(pid)?.updated_at || byIdMap.get(pid)?.created_at,
    })
    if (recentList.length >= 8) break
  }

  return (
    <div className="flex flex-col min-h-screen bg-neutral-50 dark:bg-[#141414]">
      <TopHeader
        title="Upload Dataset"
        breadcrumbs={[{ label: "Upload Dataset" }]}
        actions={
          <button
            type="button"
            onClick={() => setShowCreate(true)}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[6px] bg-[#3ecf8e] hover:bg-[#34b77c] text-[#141414] text-xs font-semibold cursor-pointer transition-colors"
          >
            <Plus className="size-3.5" />
            <span>Create Project</span>
          </button>
        }
      />

      <main className="flex-1 w-full max-w-4xl mx-auto px-4 md:px-6 py-6 space-y-6">
        {/* Unified Dataset Upload Panel */}
        <section className="rounded-lg border border-neutral-200 dark:border-[#2a2a2a] bg-white dark:bg-[#1b1b1b] p-5">
          <UnifiedDatasetUploadPanel
            workspaceId={myMaps[0]?.id || ""}
            workspaceName={myMaps[0]?.name || "Private Workspace"}
            primaryActionLabel="Open Workspace"
            showBackButton={false}
            autoAddToWorkspace
            resolveProjectId={resolveOrCreateProjectId}
            onComplete={handleUploadComplete}
          />
        </section>

        {/* Recent Projects (Shown on the Same Upload Dataset Page) */}
        <section className="rounded-lg border border-neutral-200 dark:border-[#2a2a2a] bg-white dark:bg-[#1b1b1b] overflow-hidden">
          <div className="px-4 py-3 border-b border-neutral-200 dark:border-[#262626] flex items-center justify-between">
            <div className="flex items-center gap-2 text-xs font-semibold text-neutral-900 dark:text-white">
              <Clock className="size-3.5 text-[#3ecf8e]" />
              <span>Recent Projects</span>
            </div>
            <Link
              to="/projects"
              className="inline-flex items-center gap-1 text-[11px] font-medium text-[#24b47e] dark:text-[#3ecf8e] hover:underline"
            >
              All projects <ArrowRight className="size-3" />
            </Link>
          </div>

          {projectError && (
            <div className="m-4 p-3 rounded-[6px] bg-red-500/10 border border-red-500/30 text-xs text-red-600 dark:text-red-400">
              {projectError}
            </div>
          )}

          {loadingProjects ? (
            <div className="py-10 flex items-center justify-center gap-2 text-xs text-neutral-500">
              <Loader2 className="size-4 animate-spin text-[#3ecf8e]" />
              <span>Loading recent projects…</span>
            </div>
          ) : recentList.length === 0 ? (
            <div className="p-8 text-center text-xs text-neutral-500 space-y-2">
              <FolderOpen className="size-6 mx-auto text-neutral-400" />
              <div>No projects yet. Upload a dataset above or create your first project.</div>
            </div>
          ) : (
            <div className="divide-y divide-neutral-200 dark:divide-[#262626]">
              {recentList.map((p) => (
                <button
                  key={p.id}
                  type="button"
                  onClick={() => navigate(`/maps/${encodeURIComponent(p.id)}/edit`)}
                  className="w-full px-4 py-3 text-left hover:bg-neutral-50 dark:hover:bg-[#222222] flex items-center justify-between gap-3 cursor-pointer transition-colors"
                >
                  <span className="flex items-center gap-2.5 min-w-0">
                    <Layers className="size-3.5 text-[#3ecf8e] shrink-0" />
                    <span className="text-xs font-medium text-neutral-900 dark:text-white truncate">
                      {p.name}
                    </span>
                  </span>
                  <span className="text-[11px] text-neutral-500 dark:text-neutral-400 shrink-0">
                    {formatDate(p.date)}
                  </span>
                </button>
              ))}
            </div>
          )}
        </section>
      </main>

      {/* Create Project Dialog */}
      {showCreate && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <form
            onSubmit={handleCreateProject}
            className="w-full max-w-sm rounded-lg border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#1b1b1b] p-5 space-y-4 shadow-2xl"
          >
            <div className="flex items-center justify-between">
              <h2 className="text-sm font-semibold text-neutral-900 dark:text-white">Create Project</h2>
              <button
                type="button"
                onClick={() => setShowCreate(false)}
                className="text-neutral-400 hover:text-white cursor-pointer"
              >
                <X className="size-4" />
              </button>
            </div>
            <input
              type="text"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              placeholder="Project name"
              className="w-full h-9 px-3 rounded-[6px] border border-neutral-300 dark:border-[#363636] bg-white dark:bg-[#141414] text-xs outline-none focus:border-[#3ecf8e]"
              autoFocus
            />
            <div className="flex justify-end gap-2">
              <button
                type="button"
                onClick={() => setShowCreate(false)}
                className="px-3 py-1.5 rounded-[6px] text-xs text-neutral-500 hover:bg-neutral-100 dark:hover:bg-[#262626] cursor-pointer"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={!newName.trim() || creating}
                className="px-3 py-1.5 rounded-[6px] bg-[#3ecf8e] hover:bg-[#34b77c] text-[#141414] text-xs font-semibold disabled:opacity-40 cursor-pointer"
              >
                {creating ? "Creating…" : "Create"}
              </button>
            </div>
          </form>
        </div>
      )}
    </div>
  )
}
