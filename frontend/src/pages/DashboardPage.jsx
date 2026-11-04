import React, { useState, useEffect } from "react"
import { useNavigate } from "react-router-dom"
import { useAuth } from "../context/AuthContext"
import {
  listUserProjects,
  createUserProject,
  deleteUserProject,
  updateUserProject,
  listSharedProjects,
  listUserInvitations,
  respondToInvitation,
} from "../api"
import ShareProjectModal from "../components/ShareProjectModal"

export default function DashboardPage() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()

  const [activeTab, setActiveTab] = useState("my") // "my" | "shared" | "invitations"
  const [projects, setProjects] = useState([])
  const [sharedProjects, setSharedProjects] = useState([])
  const [invitations, setInvitations] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")

  // New Project modal state
  const [showCreateModal, setShowCreateModal] = useState(false)
  const [newProjectName, setNewProjectName] = useState("")
  const [newProjectDesc, setNewProjectDesc] = useState("")
  const [creating, setCreating] = useState(false)

  // Rename modal state
  const [editingProject, setEditingProject] = useState(null)
  const [editName, setEditName] = useState("")
  const [editDesc, setEditDesc] = useState("")
  const [updating, setUpdating] = useState(false)

  // Share modal state
  const [sharingProject, setSharingProject] = useState(null)

  const fetchAllData = async () => {
    try {
      setLoading(true)
      const [owned, shared, invites] = await Promise.all([
        listUserProjects().catch(() => []),
        listSharedProjects().catch(() => []),
        listUserInvitations().catch(() => []),
      ])
      setProjects(owned)
      setSharedProjects(shared)
      setInvitations(invites)
    } catch (err) {
      setError("Failed to load your workspaces.")
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchAllData()
  }, [])

  const handleCreate = async (e) => {
    e.preventDefault()
    if (!newProjectName.trim()) return

    setCreating(true)
    try {
      const created = await createUserProject(newProjectName.trim(), newProjectDesc.trim())
      setShowCreateModal(false)
      setNewProjectName("")
      setNewProjectDesc("")
      await fetchAllData()
      navigate(`/workspace?project=${encodeURIComponent(created.id)}`)
    } catch (err) {
      alert("Failed to create project: " + (err.response?.data?.error?.message || err.message))
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (project, e) => {
    e.stopPropagation()
    if (!window.confirm(`Are you sure you want to permanently delete project "${project.name}"? All associated files and datasets will be removed.`)) {
      return
    }

    try {
      await deleteUserProject(project.id)
      setProjects((prev) => prev.filter((p) => p.id !== project.id))
    } catch (err) {
      alert("Failed to delete project: " + (err.response?.data?.error?.message || err.message))
    }
  }

  const handleOpenEdit = (project, e) => {
    e.stopPropagation()
    setEditingProject(project)
    setEditName(project.name)
    setEditDesc(project.description || "")
  }

  const handleOpenShare = (project, e) => {
    e.stopPropagation()
    setSharingProject(project)
  }

  const handleUpdate = async (e) => {
    e.preventDefault()
    if (!editName.trim() || !editingProject) return

    setUpdating(true)
    try {
      const updated = await updateUserProject(editingProject.id, {
        name: editName.trim(),
        description: editDesc.trim(),
      })
      setProjects((prev) => prev.map((p) => (p.id === updated.id ? updated : p)))
      setEditingProject(null)
    } catch (err) {
      alert("Failed to update project: " + (err.response?.data?.error?.message || err.message))
    } finally {
      setUpdating(false)
    }
  }

  const handleRespondInvite = async (invitationId, accept) => {
    try {
      await respondToInvitation(invitationId, accept)
      await fetchAllData()
    } catch (err) {
      alert("Failed to respond to invitation: " + (err.response?.data?.error?.message || err.message))
    }
  }

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100 flex flex-col">
      {/* Top Navigation Bar */}
      <header className="h-16 border-b border-slate-800 bg-slate-900/50 backdrop-blur-md px-6 flex items-center justify-between sticky top-0 z-30">
        <div className="flex items-center gap-3">
          <div className="w-8 h-8 rounded-lg bg-blue-600/20 border border-blue-500/30 flex items-center justify-center text-blue-400 font-bold">
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l4.553 2.276A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7" />
            </svg>
          </div>
          <span className="font-semibold text-lg text-white tracking-tight">Veracity Maps</span>
          <span className="text-xs px-2 py-0.5 rounded-full bg-slate-800 text-slate-400 border border-slate-700">1.3</span>
        </div>

        <div className="flex items-center gap-4">
          <div className="hidden sm:flex flex-col text-right">
            <span className="text-xs font-medium text-slate-200">{user?.name}</span>
            <span className="text-[11px] text-slate-400">{user?.email}</span>
          </div>
          <div className="w-8 h-8 rounded-full bg-blue-600/30 text-blue-300 font-semibold flex items-center justify-center text-xs uppercase border border-blue-500/20">
            {user?.name?.charAt(0) || "U"}
          </div>
          <button
            onClick={logout}
            className="px-3 py-1.5 rounded-lg text-xs font-medium bg-slate-800 hover:bg-slate-700 text-slate-300 hover:text-white border border-slate-700 transition-colors"
          >
            Log out
          </button>
        </div>
      </header>

      {/* Main Content Area */}
      <main className="flex-1 max-w-7xl w-full mx-auto px-6 py-8">
        {/* Welcome & Action Header */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
          <div>
            <h1 className="text-2xl font-bold text-white tracking-tight">Workspaces & Collaboration</h1>
            <p className="text-sm text-slate-400 mt-1">
              Manage your personal maps and collaborate on shared projects.
            </p>
          </div>
          <button
            onClick={() => setShowCreateModal(true)}
            className="inline-flex items-center gap-2 px-4 py-2.5 rounded-lg bg-blue-600 hover:bg-blue-500 text-white font-medium text-sm transition-colors shadow-lg shadow-blue-600/20"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
            </svg>
            New Project
          </button>
        </div>

        {/* Section Tabs */}
        <div className="flex items-center gap-2 border-b border-slate-800 pb-3 mb-6">
          <button
            onClick={() => setActiveTab("my")}
            className={`px-4 py-2 rounded-lg text-sm font-medium transition-all ${
              activeTab === "my"
                ? "bg-blue-600 text-white shadow-sm"
                : "text-slate-400 hover:text-white hover:bg-slate-900"
            }`}
          >
            My Projects ({projects.length})
          </button>
          <button
            onClick={() => setActiveTab("shared")}
            className={`px-4 py-2 rounded-lg text-sm font-medium transition-all ${
              activeTab === "shared"
                ? "bg-blue-600 text-white shadow-sm"
                : "text-slate-400 hover:text-white hover:bg-slate-900"
            }`}
          >
            Shared With Me ({sharedProjects.length})
          </button>
          <button
            onClick={() => setActiveTab("invitations")}
            className={`relative px-4 py-2 rounded-lg text-sm font-medium transition-all ${
              activeTab === "invitations"
                ? "bg-blue-600 text-white shadow-sm"
                : "text-slate-400 hover:text-white hover:bg-slate-900"
            }`}
          >
            Pending Invitations
            {invitations.length > 0 && (
              <span className="ml-2 px-1.5 py-0.5 rounded-full text-[10px] bg-amber-500 text-black font-bold">
                {invitations.length}
              </span>
            )}
          </button>
        </div>

        {/* Content depending on active tab */}
        {loading ? (
          <div className="flex flex-col items-center justify-center py-24 text-slate-500">
            <svg className="animate-spin h-8 w-8 text-blue-500 mb-3" fill="none" viewBox="0 0 24 24">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
            </svg>
            <span className="text-sm">Loading workspaces...</span>
          </div>
        ) : error ? (
          <div className="p-6 rounded-xl bg-red-500/10 border border-red-500/20 text-red-400 text-center">
            {error}
          </div>
        ) : activeTab === "my" ? (
          /* MY PROJECTS GRID */
          projects.length === 0 ? (
            <div className="border border-dashed border-slate-800 rounded-2xl p-12 text-center flex flex-col items-center justify-center">
              <div className="w-12 h-12 rounded-xl bg-slate-900 border border-slate-800 flex items-center justify-center text-slate-400 mb-4">
                <svg className="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 13h6m-3-3v6m-9 1V7a2 2 0 012-2h6l2 2h6a2 2 0 012 2v8a2 2 0 01-2 2H5a2 2 0 01-2-2z" />
                </svg>
              </div>
              <h3 className="text-base font-semibold text-white">No personal projects yet</h3>
              <p className="text-sm text-slate-400 max-w-sm mt-1 mb-6">
                Create a project to start uploading, georeferencing, and sharing maps.
              </p>
              <button
                onClick={() => setShowCreateModal(true)}
                className="px-4 py-2 rounded-lg bg-blue-600 hover:bg-blue-500 text-white font-medium text-sm transition-colors"
              >
                Create New Project
              </button>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
              {projects.map((project) => (
                <div
                  key={project.id}
                  onClick={() => navigate(`/workspace?project=${encodeURIComponent(project.id)}`)}
                  className="group relative bg-slate-900/60 hover:bg-slate-900 border border-slate-800 hover:border-blue-500/50 rounded-xl p-5 transition-all duration-200 cursor-pointer flex flex-col justify-between shadow-sm hover:shadow-md"
                >
                  <div>
                    <div className="flex items-start justify-between gap-3 mb-2">
                      <div className="flex items-center gap-2">
                        <span className="text-[10px] px-1.5 py-0.5 rounded bg-blue-500/10 text-blue-400 border border-blue-500/20 font-semibold uppercase">
                          Owner
                        </span>
                        <h3 className="text-base font-semibold text-white group-hover:text-blue-400 transition-colors line-clamp-1">
                          {project.name}
                        </h3>
                      </div>

                      <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                        <button
                          title="Share & Collaborators"
                          onClick={(e) => handleOpenShare(project, e)}
                          className="p-1.5 rounded hover:bg-blue-600/20 text-slate-400 hover:text-blue-400 transition-colors"
                        >
                          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8.684 13.342C8.886 12.938 9 12.482 9 12c0-.482-.114-.938-.316-1.342m0 2.684a3 3 0 110-2.684m0 2.684l6.632 3.316m-6.632-6l6.632-3.316m0 0a3 3 0 105.367-2.684 3 3 0 00-5.367 2.684zm0 9.316a3 3 0 105.368 2.684 3 3 0 00-5.368-2.684z" />
                          </svg>
                        </button>
                        <button
                          title="Rename"
                          onClick={(e) => handleOpenEdit(project, e)}
                          className="p-1.5 rounded hover:bg-slate-800 text-slate-400 hover:text-white transition-colors"
                        >
                          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                          </svg>
                        </button>
                        <button
                          title="Delete"
                          onClick={(e) => handleDelete(project, e)}
                          className="p-1.5 rounded hover:bg-red-500/20 text-slate-400 hover:text-red-400 transition-colors"
                        >
                          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                          </svg>
                        </button>
                      </div>
                    </div>

                    <p className="text-xs text-slate-400 line-clamp-2 min-h-[32px]">
                      {project.description || "No description provided."}
                    </p>
                  </div>

                  <div className="mt-6 pt-4 border-t border-slate-800/80 flex items-center justify-between text-[11px] text-slate-500">
                    <span>
                      {project.dataset_ids?.length || 0} {project.dataset_ids?.length === 1 ? "layer" : "layers"}
                    </span>
                    <span>
                      Updated {new Date(project.updated_at).toLocaleDateString()}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          )
        ) : activeTab === "shared" ? (
          /* SHARED WITH ME GRID */
          sharedProjects.length === 0 ? (
            <div className="border border-dashed border-slate-800 rounded-2xl p-12 text-center flex flex-col items-center justify-center">
              <h3 className="text-base font-semibold text-white">No shared projects</h3>
              <p className="text-sm text-slate-400 max-w-sm mt-1">
                When other users invite you to their maps, they will appear here.
              </p>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
              {sharedProjects.map(({ project, owner_name, owner_email, role, shared_date }) => (
                <div
                  key={project.id}
                  onClick={() => navigate(`/workspace?project=${encodeURIComponent(project.id)}`)}
                  className="group relative bg-slate-900/60 hover:bg-slate-900 border border-slate-800 hover:border-emerald-500/50 rounded-xl p-5 transition-all duration-200 cursor-pointer flex flex-col justify-between shadow-sm hover:shadow-md"
                >
                  <div>
                    <div className="flex items-start justify-between gap-3 mb-2">
                      <div className="flex items-center gap-2">
                        <span className={`text-[10px] px-1.5 py-0.5 rounded font-semibold uppercase ${
                          role === "editor"
                            ? "bg-emerald-500/10 text-emerald-400 border border-emerald-500/20"
                            : "bg-purple-500/10 text-purple-400 border border-purple-500/20"
                        }`}>
                          {role}
                        </span>
                        <h3 className="text-base font-semibold text-white group-hover:text-emerald-400 transition-colors line-clamp-1">
                          {project.name}
                        </h3>
                      </div>
                    </div>

                    <p className="text-xs text-slate-400 line-clamp-2 min-h-[32px]">
                      {project.description || "No description provided."}
                    </p>

                    <div className="mt-2 text-[11px] text-slate-400">
                      Owner: <span className="text-slate-300 font-medium">{owner_name}</span> ({owner_email})
                    </div>
                  </div>

                  <div className="mt-6 pt-4 border-t border-slate-800/80 flex items-center justify-between text-[11px] text-slate-500">
                    <span>
                      {project.dataset_ids?.length || 0} layers
                    </span>
                    <span>
                      Shared {new Date(shared_date).toLocaleDateString()}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          )
        ) : (
          /* PENDING INVITATIONS LIST */
          invitations.length === 0 ? (
            <div className="border border-dashed border-slate-800 rounded-2xl p-12 text-center flex flex-col items-center justify-center">
              <h3 className="text-base font-semibold text-white">No pending invitations</h3>
              <p className="text-sm text-slate-400 max-w-sm mt-1">
                You're all caught up! New invites from team members will appear here.
              </p>
            </div>
          ) : (
            <div className="space-y-3 max-w-3xl">
              {invitations.map((inv) => (
                <div
                  key={inv.invitation_id}
                  className="p-4 rounded-xl bg-slate-900 border border-slate-800 flex items-center justify-between gap-4"
                >
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="text-xs font-semibold px-2 py-0.5 rounded bg-blue-500/10 text-blue-400 border border-blue-500/20 uppercase">
                        {inv.role}
                      </span>
                      <h4 className="text-sm font-bold text-white">{inv.project_name}</h4>
                    </div>
                    <p className="text-xs text-slate-400 mt-1">
                      Invited by <span className="text-slate-200 font-medium">{inv.inviter_name}</span> on {new Date(inv.created_at).toLocaleDateString()}
                    </p>
                  </div>

                  <div className="flex items-center gap-2 shrink-0">
                    <button
                      onClick={() => handleRespondInvite(inv.invitation_id, true)}
                      className="px-3.5 py-1.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-medium transition-colors"
                    >
                      Accept
                    </button>
                    <button
                      onClick={() => handleRespondInvite(inv.invitation_id, false)}
                      className="px-3.5 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 hover:text-white text-xs font-medium transition-colors"
                    >
                      Decline
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )
        )}
      </main>

      {/* Share Project Modal */}
      {sharingProject && (
        <ShareProjectModal
          project={sharingProject}
          isOpen={!!sharingProject}
          onClose={() => setSharingProject(null)}
        />
      )}

      {/* New Project Modal */}
      {showCreateModal && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-md w-full p-6 shadow-2xl">
            <h2 className="text-lg font-bold text-white mb-1">Create New Project</h2>
            <p className="text-xs text-slate-400 mb-4">Set up a private workspace container for your maps.</p>

            <form onSubmit={handleCreate} className="space-y-4">
              <div>
                <label className="block text-xs font-medium text-slate-300 uppercase tracking-wider mb-1.5">
                  Project Name
                </label>
                <input
                  type="text"
                  required
                  autoFocus
                  value={newProjectName}
                  onChange={(e) => setNewProjectName(e.target.value)}
                  placeholder="e.g. Downtown Facility Revitalization"
                  className="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white text-sm focus:outline-none focus:border-blue-500"
                />
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-300 uppercase tracking-wider mb-1.5">
                  Description (Optional)
                </label>
                <textarea
                  rows={3}
                  value={newProjectDesc}
                  onChange={(e) => setNewProjectDesc(e.target.value)}
                  placeholder="Notes, geographic location, or project scope..."
                  className="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white text-sm focus:outline-none focus:border-blue-500 resize-none"
                />
              </div>

              <div className="flex items-center justify-end gap-3 pt-2">
                <button
                  type="button"
                  onClick={() => setShowCreateModal(false)}
                  className="px-4 py-2 rounded-lg text-sm text-slate-300 hover:bg-slate-800 transition-colors"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={creating || !newProjectName.trim()}
                  className="px-4 py-2 rounded-lg bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium transition-colors disabled:opacity-50"
                >
                  {creating ? "Creating..." : "Create Project"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Edit Project Modal */}
      {editingProject && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-md w-full p-6 shadow-2xl">
            <h2 className="text-lg font-bold text-white mb-1">Edit Project</h2>
            <p className="text-xs text-slate-400 mb-4">Update the name and description of this project.</p>

            <form onSubmit={handleUpdate} className="space-y-4">
              <div>
                <label className="block text-xs font-medium text-slate-300 uppercase tracking-wider mb-1.5">
                  Project Name
                </label>
                <input
                  type="text"
                  required
                  autoFocus
                  value={editName}
                  onChange={(e) => setEditName(e.target.value)}
                  className="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white text-sm focus:outline-none focus:border-blue-500"
                />
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-300 uppercase tracking-wider mb-1.5">
                  Description
                </label>
                <textarea
                  rows={3}
                  value={editDesc}
                  onChange={(e) => setEditDesc(e.target.value)}
                  className="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white text-sm focus:outline-none focus:border-blue-500 resize-none"
                />
              </div>

              <div className="flex items-center justify-end gap-3 pt-2">
                <button
                  type="button"
                  onClick={() => setEditingProject(null)}
                  className="px-4 py-2 rounded-lg text-sm text-slate-300 hover:bg-slate-800 transition-colors"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={updating || !editName.trim()}
                  className="px-4 py-2 rounded-lg bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium transition-colors disabled:opacity-50"
                >
                  {updating ? "Saving..." : "Save Changes"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}
