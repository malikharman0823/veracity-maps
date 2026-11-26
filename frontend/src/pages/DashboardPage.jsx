import React, { useEffect, useState, useRef } from "react"
import { Link, useNavigate, useSearchParams } from "react-router-dom"
import {
  Plus,
  Upload,
  FolderOpen,
  Users,
  Clock,
  ArrowRight,
  Loader2,
  X,
} from "lucide-react"
import TopHeader from "../components/TopHeader"
import { useAuth } from "../context/AuthContext"
import { listMaps, createMap, getMapInvitation, acceptMapInvitation } from "../api"

function formatDate(val) {
  if (!val) return "—"
  const d = new Date(val)
  if (isNaN(d.getTime()) || d.getFullYear() <= 1) return "—"
  return d.toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" })
}

export default function DashboardPage() {
  const { user } = useAuth()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()

  const [myMaps, setMyMaps] = useState([])
  const [sharedMaps, setSharedMaps] = useState([])
  const [recentMaps, setRecentMaps] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [notice, setNotice] = useState("")

  const [showCreate, setShowCreate] = useState(false)
  const [newName, setNewName] = useState("")
  const [creating, setCreating] = useState(false)
  const acceptedRef = useRef(new Set())

  const fetchMaps = async () => {
    setLoading(true)
    setError("")
    try {
      const data = await listMaps()
      const my = data?.my_maps || []
      const shared = data?.shared_with_me || []
      const recent = data?.recent_maps || []
      setMyMaps(my)
      setSharedMaps(shared)
      setRecentMaps(recent)
      window.dispatchEvent(
        new CustomEvent("veracity:maps-updated", { detail: { myMaps: my, sharedWithMe: shared } })
      )
    } catch {
      setError("Unable to load dashboard projects.")
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchMaps()
  }, [])

  // Preserve email invitation token acceptance flow
  useEffect(() => {
    const inviteToken = searchParams.get("invite")
    if (!inviteToken || acceptedRef.current.has(inviteToken)) return
    acceptedRef.current.add(inviteToken)
    ;(async () => {
      try {
        await getMapInvitation(inviteToken)
        const res = await acceptMapInvitation(inviteToken)
        setNotice("Invitation accepted.")
        setSearchParams({}, { replace: true })
        await fetchMaps()
        const targetMapId = res?.map_id || res?.map?.id
        if (targetMapId) navigate(`/maps/${targetMapId}/edit`)
      } catch {
        setError("Invitation link is invalid or expired.")
      }
    })()
  }, [searchParams, setSearchParams, navigate])

  const handleCreateProject = async (e) => {
    e.preventDefault()
    if (!newName.trim() || creating) return
    setCreating(true)
    try {
      const res = await createMap({ name: newName.trim() })
      const created = res?.map || res
      setNewName("")
      setShowCreate(false)
      await fetchMaps()
      if (created?.id) navigate(`/maps/${created.id}/edit`)
    } catch {
      setError("Failed to create project.")
    } finally {
      setCreating(false)
    }
  }

  const recentList = (recentMaps.length > 0 ? recentMaps : myMaps).slice(0, 6)
  const sharedList = sharedMaps.slice(0, 6)

  return (
    <div className="flex flex-col min-h-screen bg-neutral-50 dark:bg-[#141414]">
      <TopHeader
        title="Dashboard"
        breadcrumbs={[{ label: "Dashboard" }]}
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

      <main className="flex-1 w-full max-w-5xl mx-auto px-4 md:px-6 py-6 space-y-6">
        {/* Welcome Section */}
        <section className="rounded-lg border border-neutral-200 dark:border-[#2a2a2a] bg-white dark:bg-[#1b1b1b] p-5 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div>
            <h2 className="text-base font-semibold text-neutral-900 dark:text-white">
              Welcome back{user?.name ? `, ${user.name}` : ""}
            </h2>
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
              Create a project or upload geospatial data to start mapping.
            </p>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <Link
              to="/upload"
              className="inline-flex items-center gap-1.5 px-3 py-2 rounded-[6px] border border-neutral-200 dark:border-[#323232] bg-white dark:bg-[#222222] hover:border-[#3ecf8e] text-xs font-medium text-neutral-700 dark:text-neutral-200 transition-colors"
            >
              <Upload className="size-3.5" />
              <span>Upload Data</span>
            </Link>
            <button
              type="button"
              onClick={() => setShowCreate(true)}
              className="inline-flex items-center gap-1.5 px-3 py-2 rounded-[6px] bg-[#3ecf8e] hover:bg-[#34b77c] text-[#141414] text-xs font-semibold cursor-pointer transition-colors"
            >
              <Plus className="size-3.5" />
              <span>Create Project</span>
            </button>
          </div>
        </section>

        {notice && (
          <div className="p-3 rounded-[6px] bg-[#3ecf8e]/10 border border-[#3ecf8e]/30 text-xs text-[#24b47e] dark:text-[#3ecf8e]">
            {notice}
          </div>
        )}
        {error && (
          <div className="p-3 rounded-[6px] bg-red-500/10 border border-red-500/30 text-xs text-red-600 dark:text-red-400">
            {error}
          </div>
        )}

        {loading ? (
          <div className="py-12 flex items-center justify-center gap-2 text-xs text-neutral-500">
            <Loader2 className="size-4 animate-spin text-[#3ecf8e]" />
            <span>Loading workspace…</span>
          </div>
        ) : (
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            {/* Recent Projects */}
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
                  View all <ArrowRight className="size-3" />
                </Link>
              </div>
              {recentList.length === 0 ? (
                <div className="p-8 text-center text-xs text-neutral-500 space-y-2">
                  <FolderOpen className="size-6 mx-auto text-neutral-400" />
                  <div>No projects yet.</div>
                </div>
              ) : (
                <div className="divide-y divide-neutral-200 dark:divide-[#262626]">
                  {recentList.map((m) => (
                    <button
                      key={m.id}
                      type="button"
                      onClick={() => navigate(`/maps/${m.id}/edit`)}
                      className="w-full px-4 py-3 text-left hover:bg-neutral-50 dark:hover:bg-[#222222] flex items-center justify-between gap-3 cursor-pointer transition-colors"
                    >
                      <span className="text-xs font-medium text-neutral-900 dark:text-white truncate">
                        {m.name}
                      </span>
                      <span className="text-[11px] text-neutral-500 dark:text-neutral-400 shrink-0">
                        {formatDate(m.updated_at || m.created_at)}
                      </span>
                    </button>
                  ))}
                </div>
              )}
            </section>

            {/* Shared Projects */}
            <section className="rounded-lg border border-neutral-200 dark:border-[#2a2a2a] bg-white dark:bg-[#1b1b1b] overflow-hidden">
              <div className="px-4 py-3 border-b border-neutral-200 dark:border-[#262626] flex items-center justify-between">
                <div className="flex items-center gap-2 text-xs font-semibold text-neutral-900 dark:text-white">
                  <Users className="size-3.5 text-sky-500" />
                  <span>Shared Projects</span>
                </div>
                <Link
                  to="/shared"
                  className="inline-flex items-center gap-1 text-[11px] font-medium text-[#24b47e] dark:text-[#3ecf8e] hover:underline"
                >
                  View all <ArrowRight className="size-3" />
                </Link>
              </div>
              {sharedList.length === 0 ? (
                <div className="p-8 text-center text-xs text-neutral-500 space-y-2">
                  <Users className="size-6 mx-auto text-neutral-400" />
                  <div>No projects shared with you yet.</div>
                </div>
              ) : (
                <div className="divide-y divide-neutral-200 dark:divide-[#262626]">
                  {sharedList.map((m) => (
                    <button
                      key={m.id}
                      type="button"
                      onClick={() => navigate(`/maps/${m.id}/edit`)}
                      className="w-full px-4 py-3 text-left hover:bg-neutral-50 dark:hover:bg-[#222222] flex items-center justify-between gap-3 cursor-pointer transition-colors"
                    >
                      <span className="text-xs font-medium text-neutral-900 dark:text-white truncate">
                        {m.name}
                      </span>
                      <span className="px-1.5 py-0.5 rounded bg-neutral-100 dark:bg-[#2a2a2a] text-[10px] font-medium uppercase text-neutral-500 dark:text-neutral-300 shrink-0">
                        {m.permission || "view"}
                      </span>
                    </button>
                  ))}
                </div>
              )}
            </section>
          </div>
        )}
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
    </div>
  )
}
