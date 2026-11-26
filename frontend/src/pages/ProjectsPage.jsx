import React, { useEffect, useMemo, useState } from "react"
import { useNavigate } from "react-router-dom"
import {
  Plus,
  Search,
  MoreHorizontal,
  Edit2,
  Share2,
  Trash2,
  FolderOpen,
  Users,
  ArrowUpDown,
  Loader2,
  X,
} from "lucide-react"
import TopHeader from "../components/TopHeader"
import ShareMapModal from "../components/ShareMapModal"
import { listMaps, createMap, updateMap, deleteMap } from "../api"

function formatDate(val) {
  if (!val) return "—"
  const d = new Date(val)
  if (isNaN(d.getTime()) || d.getFullYear() <= 1) return "—"
  return d.toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" })
}

export default function ProjectsPage({ mode = "projects" }) {
  const isSharedView = mode === "shared"
  const navigate = useNavigate()

  const [myMaps, setMyMaps] = useState([])
  const [sharedMaps, setSharedMaps] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [search, setSearch] = useState("")
  const [sortBy, setSortBy] = useState("updated") // "updated" | "name"

  const [menuOpenId, setMenuOpenId] = useState(null)
  const [showCreate, setShowCreate] = useState(false)
  const [newName, setNewName] = useState("")
  const [creating, setCreating] = useState(false)

  const [renamingMap, setRenamingMap] = useState(null)
  const [renameDraft, setRenameDraft] = useState("")
  const [sharingMap, setSharingMap] = useState(null)

  const fetchAll = async () => {
    setLoading(true)
    setError("")
    try {
      const data = await listMaps()
      const my = data?.my_maps || []
      const shared = data?.shared_with_me || []
      setMyMaps(my)
      setSharedMaps(shared)
      window.dispatchEvent(new CustomEvent("veracity:maps-updated", { detail: { myMaps: my, sharedWithMe: shared } }))
    } catch {
      setError("Unable to load projects.")
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchAll()
  }, [])

  const items = useMemo(() => {
    const base = isSharedView ? sharedMaps : myMaps
    const q = search.trim().toLowerCase()
    const filtered = q ? base.filter((m) => (m.name || "").toLowerCase().includes(q)) : [...base]
    filtered.sort((a, b) => {
      if (sortBy === "name") return (a.name || "").localeCompare(b.name || "")
      return new Date(b.updated_at || b.created_at || 0) - new Date(a.updated_at || a.created_at || 0)
    })
    return filtered
  }, [isSharedView, myMaps, sharedMaps, search, sortBy])

  const handleCreate = async (e) => {
    e.preventDefault()
    if (!newName.trim() || creating) return
    setCreating(true)
    try {
      const res = await createMap({ name: newName.trim() })
      const created = res?.map || res
      setNewName("")
      setShowCreate(false)
      await fetchAll()
      if (created?.id) navigate(`/maps/${created.id}/edit`)
    } catch {
      setError("Failed to create project.")
    } finally {
      setCreating(false)
    }
  }

  const handleRename = async (e) => {
    e.preventDefault()
    if (!renamingMap || !renameDraft.trim()) return
    try {
      await updateMap(renamingMap.id, { name: renameDraft.trim() })
      setRenamingMap(null)
      await fetchAll()
    } catch {
      setError("Failed to rename project.")
    }
  }

  const handleDelete = async (m) => {
    setMenuOpenId(null)
    if (!window.confirm(`Delete project "${m.name}"? This cannot be undone.`)) return
    try {
      await deleteMap(m.id)
      await fetchAll()
    } catch {
      setError("Failed to delete project.")
    }
  }

  return (
    <div
      className="flex flex-col min-h-screen bg-neutral-50 dark:bg-[#141414]"
      onClick={() => menuOpenId && setMenuOpenId(null)}
    >
      <TopHeader
        title={isSharedView ? "Shared Projects" : "Projects"}
        breadcrumbs={[{ label: isSharedView ? "Shared Projects" : "Projects" }]}
        actions={
          !isSharedView ? (
            <button
              type="button"
              onClick={() => setShowCreate(true)}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[6px] bg-[#3ecf8e] hover:bg-[#34b77c] text-[#141414] text-xs font-semibold cursor-pointer transition-colors"
            >
              <Plus className="size-3.5" />
              <span>Create Project</span>
            </button>
          ) : null
        }
      />

      <main className="flex-1 w-full max-w-5xl mx-auto px-4 md:px-6 py-6 space-y-4">
        {/* Search & Sort Bar */}
        <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2.5">
          <div className="relative flex-1">
            <Search className="size-3.5 text-neutral-400 absolute left-3 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={isSharedView ? "Search shared projects…" : "Search projects…"}
              className="w-full h-9 pl-8 pr-3 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#1b1b1b] text-xs outline-none focus:border-[#3ecf8e]"
            />
          </div>
          <button
            type="button"
            onClick={() => setSortBy((s) => (s === "updated" ? "name" : "updated"))}
            className="inline-flex items-center justify-center gap-1.5 h-9 px-3 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#1b1b1b] text-xs font-medium text-neutral-700 dark:text-neutral-200 hover:border-[#3ecf8e] cursor-pointer"
          >
            <ArrowUpDown className="size-3.5 text-neutral-400" />
            <span>Sort: {sortBy === "updated" ? "Last Modified" : "Name"}</span>
          </button>
        </div>

        {error && (
          <div className="p-3 rounded-[6px] bg-red-500/10 border border-red-500/30 text-xs text-red-600 dark:text-red-400">
            {error}
          </div>
        )}

        {loading ? (
          <div className="py-16 flex items-center justify-center gap-2 text-xs text-neutral-500">
            <Loader2 className="size-4 animate-spin text-[#3ecf8e]" />
            <span>Loading projects…</span>
          </div>
        ) : items.length === 0 ? (
          <div className="rounded-lg border border-dashed border-neutral-300 dark:border-[#303030] bg-white dark:bg-[#1b1b1b] p-12 text-center space-y-2">
            {isSharedView ? (
              <Users className="size-7 text-neutral-400 mx-auto" />
            ) : (
              <FolderOpen className="size-7 text-neutral-400 mx-auto" />
            )}
            <div className="text-sm font-medium text-neutral-800 dark:text-neutral-200">
              {search ? "No matching projects" : isSharedView ? "No shared projects yet" : "No projects yet"}
            </div>
            {!isSharedView && !search && (
              <button
                type="button"
                onClick={() => setShowCreate(true)}
                className="mt-2 inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[6px] bg-[#3ecf8e] text-[#141414] text-xs font-semibold cursor-pointer"
              >
                <Plus className="size-3.5" /> Create Your First Project
              </button>
            )}
          </div>
        ) : (
          <div className="rounded-lg border border-neutral-200 dark:border-[#2a2a2a] bg-white dark:bg-[#1b1b1b] divide-y divide-neutral-200 dark:divide-[#262626]">
            {items.map((m) => (
              <div
                key={m.id}
                onClick={() => navigate(`/maps/${m.id}/edit`)}
                className="flex items-center justify-between gap-3 px-4 py-3 hover:bg-neutral-50 dark:hover:bg-[#222222] cursor-pointer transition-colors"
              >
                <div className="min-w-0 flex-1">
                  <div className="text-xs font-semibold text-neutral-900 dark:text-white truncate">
                    {m.name}
                  </div>
                  <div className="text-[11px] text-neutral-500 dark:text-neutral-400 flex items-center gap-2 mt-0.5">
                    <span>Updated {formatDate(m.updated_at || m.created_at)}</span>
                    {isSharedView && m.permission && (
                      <span className="px-1.5 py-0.2 rounded bg-neutral-100 dark:bg-[#2a2a2a] uppercase text-[10px] font-medium">
                        {m.permission}
                      </span>
                    )}
                  </div>
                </div>

                {!isSharedView && (
                  <div className="relative" onClick={(e) => e.stopPropagation()}>
                    <button
                      type="button"
                      onClick={() => setMenuOpenId(menuOpenId === m.id ? null : m.id)}
                      className="p-1.5 rounded-[6px] hover:bg-neutral-200 dark:hover:bg-[#303030] text-neutral-500 cursor-pointer"
                      aria-label="Project options"
                    >
                      <MoreHorizontal className="size-4" />
                    </button>
                    {menuOpenId === m.id && (
                      <div className="absolute right-0 mt-1 w-36 rounded-[6px] border border-neutral-200 dark:border-[#343434] bg-white dark:bg-[#222222] shadow-xl py-1 z-30 text-xs">
                        <button
                          type="button"
                          onClick={() => {
                            setMenuOpenId(null)
                            setRenamingMap(m)
                            setRenameDraft(m.name || "")
                          }}
                          className="w-full px-3 py-1.5 text-left hover:bg-neutral-100 dark:hover:bg-[#2d2d2d] flex items-center gap-2 cursor-pointer"
                        >
                          <Edit2 className="size-3.5 text-neutral-400" /> Rename
                        </button>
                        <button
                          type="button"
                          onClick={() => {
                            setMenuOpenId(null)
                            setSharingMap(m)
                          }}
                          className="w-full px-3 py-1.5 text-left hover:bg-neutral-100 dark:hover:bg-[#2d2d2d] flex items-center gap-2 cursor-pointer"
                        >
                          <Share2 className="size-3.5 text-neutral-400" /> Share
                        </button>
                        <button
                          type="button"
                          onClick={() => handleDelete(m)}
                          className="w-full px-3 py-1.5 text-left text-red-500 hover:bg-red-500/10 flex items-center gap-2 cursor-pointer"
                        >
                          <Trash2 className="size-3.5" /> Delete
                        </button>
                      </div>
                    )}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </main>

      {/* Create Project Dialog */}
      {showCreate && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <form
            onSubmit={handleCreate}
            className="w-full max-w-sm rounded-lg border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#1b1b1b] p-5 space-y-4 shadow-2xl"
          >
            <div className="flex items-center justify-between">
              <h2 className="text-sm font-semibold text-neutral-900 dark:text-white">Create Project</h2>
              <button type="button" onClick={() => setShowCreate(false)} className="text-neutral-400 hover:text-white cursor-pointer">
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

      {/* Rename Project Dialog */}
      {renamingMap && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <form
            onSubmit={handleRename}
            className="w-full max-w-sm rounded-lg border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#1b1b1b] p-5 space-y-4 shadow-2xl"
          >
            <h2 className="text-sm font-semibold text-neutral-900 dark:text-white">Rename Project</h2>
            <input
              type="text"
              value={renameDraft}
              onChange={(e) => setRenameDraft(e.target.value)}
              className="w-full h-9 px-3 rounded-[6px] border border-neutral-300 dark:border-[#363636] bg-white dark:bg-[#141414] text-xs outline-none focus:border-[#3ecf8e]"
              autoFocus
            />
            <div className="flex justify-end gap-2">
              <button
                type="button"
                onClick={() => setRenamingMap(null)}
                className="px-3 py-1.5 rounded-[6px] text-xs text-neutral-500 hover:bg-neutral-100 dark:hover:bg-[#262626] cursor-pointer"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={!renameDraft.trim()}
                className="px-3 py-1.5 rounded-[6px] bg-[#3ecf8e] text-[#141414] text-xs font-semibold cursor-pointer"
              >
                Save
              </button>
            </div>
          </form>
        </div>
      )}

      <ShareMapModal isOpen={Boolean(sharingMap)} onClose={() => setSharingMap(null)} map={sharingMap} />
    </div>
  )
}
