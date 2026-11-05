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
  listOrgInvitations,
  respondToInvitation,
  respondToOrgInvitation,
  createOrganization,
  listOrganizations,
  listProjectActivities,
  listOrgActivities,
} from "../api"
import ShareProjectModal from "../components/ShareProjectModal"
import ThemeToggle from "../components/ThemeToggle"

export default function DashboardPage() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()

  const [activeTab, setActiveTab] = useState("my") // "my" | "shared" | "organizations" | "invitations" | "activity" | "settings"
  const [projects, setProjects] = useState([])
  const [sharedProjects, setSharedProjects] = useState([])
  const [organizations, setOrganizations] = useState([])
  const [invitations, setInvitations] = useState([])
  const [activities, setActivities] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")

  // New Project modal state
  const [showCreateModal, setShowCreateModal] = useState(false)
  const [newProjectName, setNewProjectName] = useState("")
  const [newProjectDesc, setNewProjectDesc] = useState("")
  const [creating, setCreating] = useState(false)

  // New Organization modal state
  const [showCreateOrgModal, setShowCreateOrgModal] = useState(false)
  const [newOrgName, setNewOrgName] = useState("")
  const [newOrgDesc, setNewOrgDesc] = useState("")
  const [creatingOrg, setCreatingOrg] = useState(false)

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
      const [owned, shared, orgs, projInvites, orgInvites] = await Promise.all([
        listUserProjects().catch(() => []),
        listSharedProjects().catch(() => []),
        listOrganizations().catch(() => []),
        listUserInvitations().catch(() => []),
        listOrgInvitations().catch(() => []),
      ])
      setProjects(owned || [])
      setSharedProjects(shared || [])
      setOrganizations(orgs || [])

      const formattedProjInvites = (projInvites || []).map((inv) => ({ ...inv, inviteType: "project" }))
      const formattedOrgInvites = (orgInvites || []).map((inv) => ({
        ...inv,
        inviteType: "organization",
        project_name: inv.org_name,
      }))
      setInvitations([...formattedProjInvites, ...formattedOrgInvites])

      // Gather recent activities across projects and organizations
      try {
        const projActPromises = (owned || []).slice(0, 5).map((p) =>
          listProjectActivities(p.id)
            .then((acts) => (acts || []).map((a) => ({ ...a, source: p.name, sourceType: "project" })))
            .catch(() => [])
        )
        const orgActPromises = (orgs || []).slice(0, 5).map((o) =>
          listOrgActivities(o.id)
            .then((acts) => (acts || []).map((a) => ({ ...a, source: o.name, sourceType: "organization" })))
            .catch(() => [])
        )
        const actArrays = await Promise.all([...projActPromises, ...orgActPromises])
        const mergedActs = actArrays.flat().sort((a, b) => new Date(b.created_at) - new Date(a.created_at))
        setActivities(mergedActs)
      } catch {
        setActivities([])
      }
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

  const handleRespondInvite = async (inv, accept) => {
    try {
      if (inv.inviteType === "organization") {
        await respondToOrgInvitation(inv.invitation_id || inv.id, accept)
      } else {
        await respondToInvitation(inv.invitation_id || inv.id, accept)
      }
      await fetchAllData()
    } catch (err) {
      alert("Failed to respond to invitation: " + (err.response?.data?.error?.message || err.message))
    }
  }

  const handleCreateOrg = async (e) => {
    e.preventDefault()
    if (!newOrgName.trim()) return

    setCreatingOrg(true)
    try {
      const created = await createOrganization(newOrgName.trim(), newOrgDesc.trim())
      setShowCreateOrgModal(false)
      setNewOrgName("")
      setNewOrgDesc("")
      navigate(`/organizations/${encodeURIComponent(created.id)}`)
    } catch (err) {
      alert("Failed to create organization: " + (err.response?.data?.error?.message || err.message))
    } finally {
      setCreatingOrg(false)
    }
  }

  return (
    <div className="min-h-screen bg-[#ffffff] dark:bg-[#1c1c1c] text-[#171717] dark:text-[#ffffff] flex flex-col transition-colors">
      {/* Top Navigation Bar */}
      <header className="h-16 border-b border-neutral-200 dark:border-[#2e2e2e] bg-white/80 dark:bg-[#1c1c1c]/80 backdrop-blur-md px-6 flex items-center justify-between sticky top-0 z-30">
        <div className="flex items-center gap-3">
          <div className="w-8 h-8 rounded-[6px] bg-[#3ecf8e]/10 border border-[#3ecf8e]/30 flex items-center justify-center text-[#24b47e] dark:text-[#3ecf8e] font-bold">
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l4.553 2.276A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7" />
            </svg>
          </div>
          <span className="font-semibold text-lg text-neutral-900 dark:text-white tracking-tight">Veracity Maps</span>
          <span className="text-xs px-2 py-0.5 rounded-[4px] bg-neutral-100 dark:bg-[#202020] text-neutral-600 dark:text-neutral-400 border border-neutral-200 dark:border-[#2e2e2e]">1.3</span>
        </div>

        <div className="flex items-center gap-3">
          <ThemeToggle />
          <div className="hidden sm:flex flex-col text-right">
            <span className="text-xs font-medium text-neutral-800 dark:text-neutral-200">{user?.name}</span>
            <span className="text-[11px] text-neutral-500 dark:text-neutral-400">{user?.email}</span>
          </div>
          <div className="w-8 h-8 rounded-full bg-[#3ecf8e]/20 text-[#171717] dark:text-[#3ecf8e] font-semibold flex items-center justify-center text-xs uppercase border border-[#3ecf8e]/30">
            {user?.name?.charAt(0) || "U"}
          </div>
          <button
            onClick={logout}
            className="px-3 py-1.5 rounded-[6px] text-xs font-medium border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] text-neutral-700 dark:text-neutral-300 hover:text-red-500 dark:hover:text-red-400 hover:border-red-500/30 transition-colors cursor-pointer"
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
            <h1 className="text-2xl font-bold text-neutral-900 dark:text-white tracking-tight">Workspaces & Collaboration</h1>
            <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
              Manage your personal maps and collaborate on shared projects.
            </p>
          </div>
          <div className="flex items-center gap-2.5">
            <button
              onClick={() => setShowCreateOrgModal(true)}
              className="inline-flex items-center gap-2 px-3.5 py-2 rounded-[6px] bg-white dark:bg-[#202020] hover:bg-neutral-100 dark:hover:bg-[#282828] text-neutral-800 dark:text-neutral-200 border border-neutral-300 dark:border-[#2e2e2e] font-medium text-xs transition-colors cursor-pointer shadow-xs"
            >
              <svg className="w-3.5 h-3.5 text-[#24b47e] dark:text-[#3ecf8e]" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" />
              </svg>
              New Organization
            </button>
            <button
              onClick={() => setShowCreateModal(true)}
              className="inline-flex items-center gap-2 px-4 py-2 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-medium text-sm transition-colors shadow-xs cursor-pointer"
            >
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
              </svg>
              New Project
            </button>
          </div>
        </div>

        {/* Section Tabs */}
        <div className="flex items-center gap-1.5 border-b border-neutral-200 dark:border-[#2e2e2e] pb-3 mb-6 overflow-x-auto">
          <button
            onClick={() => setActiveTab("my")}
            className={`px-3.5 py-1.5 rounded-[6px] text-xs font-medium transition-all shrink-0 cursor-pointer ${
              activeTab === "my"
                ? "bg-[#3ecf8e] text-[#171717] shadow-xs"
                : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#242424]"
            }`}
          >
            My Projects ({projects.length})
          </button>
          <button
            onClick={() => setActiveTab("shared")}
            className={`px-3.5 py-1.5 rounded-[6px] text-xs font-medium transition-all shrink-0 cursor-pointer ${
              activeTab === "shared"
                ? "bg-[#3ecf8e] text-[#171717] shadow-xs"
                : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#242424]"
            }`}
          >
            Shared Projects ({sharedProjects.length})
          </button>
          <button
            onClick={() => setActiveTab("organizations")}
            className={`px-3.5 py-1.5 rounded-[6px] text-xs font-medium transition-all shrink-0 cursor-pointer ${
              activeTab === "organizations"
                ? "bg-[#3ecf8e] text-[#171717] shadow-xs"
                : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#242424]"
            }`}
          >
            Organizations ({organizations.length})
          </button>
          <button
            onClick={() => setActiveTab("invitations")}
            className={`relative px-3.5 py-1.5 rounded-[6px] text-xs font-medium transition-all shrink-0 cursor-pointer ${
              activeTab === "invitations"
                ? "bg-[#3ecf8e] text-[#171717] shadow-xs"
                : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#242424]"
            }`}
          >
            Invitations
            {invitations.length > 0 && (
              <span className="ml-1.5 px-1.5 py-0.2 rounded-full text-[10px] bg-[#ffdb13] text-[#171717] font-bold">
                {invitations.length}
              </span>
            )}
          </button>
          <button
            onClick={() => setActiveTab("activity")}
            className={`px-3.5 py-1.5 rounded-[6px] text-xs font-medium transition-all shrink-0 cursor-pointer ${
              activeTab === "activity"
                ? "bg-[#3ecf8e] text-[#171717] shadow-xs"
                : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#242424]"
            }`}
          >
            Recent Activity
          </button>
          <button
            onClick={() => setActiveTab("settings")}
            className={`px-3.5 py-1.5 rounded-[6px] text-xs font-medium transition-all shrink-0 cursor-pointer ${
              activeTab === "settings"
                ? "bg-[#3ecf8e] text-[#171717] shadow-xs"
                : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#242424]"
            }`}
          >
            Account Settings
          </button>
        </div>

        {/* Content depending on active tab */}
        {loading ? (
          <div className="flex flex-col items-center justify-center py-24 text-neutral-500">
            <svg className="animate-spin h-8 w-8 text-[#3ecf8e] mb-3" fill="none" viewBox="0 0 24 24">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
            </svg>
            <span className="text-sm">Loading workspaces...</span>
          </div>
        ) : error ? (
          <div className="p-6 rounded-[12px] bg-red-500/10 border border-red-500/20 text-red-500 dark:text-red-400 text-center text-sm">
            {error}
          </div>
        ) : activeTab === "my" ? (
          /* MY PROJECTS GRID */
          projects.length === 0 ? (
            <div className="border border-dashed border-neutral-300 dark:border-[#2e2e2e] rounded-[12px] p-12 text-center flex flex-col items-center justify-center">
              <div className="w-12 h-12 rounded-[8px] bg-neutral-100 dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-center text-neutral-400 mb-4">
                <svg className="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 13h6m-3-3v6m-9 1V7a2 2 0 012-2h6l2 2h6a2 2 0 012 2v8a2 2 0 01-2 2H5a2 2 0 01-2-2z" />
                </svg>
              </div>
              <h3 className="text-base font-semibold text-neutral-900 dark:text-white">No personal projects yet</h3>
              <p className="text-sm text-neutral-500 dark:text-neutral-400 max-w-sm mt-1 mb-6">
                Create a project to start uploading, georeferencing, and sharing maps.
              </p>
              <button
                onClick={() => setShowCreateModal(true)}
                className="px-4 py-2 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-medium text-sm transition-colors cursor-pointer shadow-xs"
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
                  className="group relative bg-white dark:bg-[#202020] hover:bg-neutral-50 dark:hover:bg-[#242424] border border-neutral-200 dark:border-[#2e2e2e] hover:border-[#3ecf8e] dark:hover:border-[#3ecf8e] rounded-[12px] p-5 transition-all duration-200 cursor-pointer flex flex-col justify-between shadow-xs hover:shadow-md"
                >
                  <div>
                    <div className="flex items-start justify-between gap-3 mb-2">
                      <div className="flex items-center gap-2">
                        <span className="text-[10px] px-1.5 py-0.5 rounded-[4px] bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30 font-semibold uppercase">
                          Owner
                        </span>
                        <h3 className="text-base font-semibold text-neutral-900 dark:text-white group-hover:text-[#24b47e] dark:group-hover:text-[#3ecf8e] transition-colors line-clamp-1">
                          {project.name}
                        </h3>
                      </div>

                      <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                        <button
                          title="Share & Collaborators"
                          onClick={(e) => handleOpenShare(project, e)}
                          className="p-1.5 rounded-[4px] hover:bg-neutral-100 dark:hover:bg-[#2e2e2e] text-neutral-500 hover:text-[#3ecf8e] transition-colors"
                        >
                          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8.684 13.342C8.886 12.938 9 12.482 9 12c0-.482-.114-.938-.316-1.342m0 2.684a3 3 0 110-2.684m0 2.684l6.632 3.316m-6.632-6l6.632-3.316m0 0a3 3 0 105.367-2.684 3 3 0 00-5.367 2.684zm0 9.316a3 3 0 105.368 2.684 3 3 0 00-5.368-2.684z" />
                          </svg>
                        </button>
                        <button
                          title="Rename"
                          onClick={(e) => handleOpenEdit(project, e)}
                          className="p-1.5 rounded-[4px] hover:bg-neutral-100 dark:hover:bg-[#2e2e2e] text-neutral-500 hover:text-neutral-900 dark:hover:text-white transition-colors"
                        >
                          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                          </svg>
                        </button>
                        <button
                          title="Delete"
                          onClick={(e) => handleDelete(project, e)}
                          className="p-1.5 rounded-[4px] hover:bg-red-500/10 text-neutral-500 hover:text-red-500 transition-colors"
                        >
                          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                          </svg>
                        </button>
                      </div>
                    </div>

                    <p className="text-xs text-neutral-500 dark:text-neutral-400 line-clamp-2 min-h-[32px]">
                      {project.description || "No description provided."}
                    </p>
                  </div>

                  <div className="mt-6 pt-4 border-t border-neutral-100 dark:border-[#2e2e2e] flex items-center justify-between text-[11px] text-neutral-500 dark:text-neutral-400">
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
            <div className="border border-dashed border-neutral-300 dark:border-[#2e2e2e] rounded-[12px] p-12 text-center flex flex-col items-center justify-center">
              <h3 className="text-base font-semibold text-neutral-900 dark:text-white">No shared projects</h3>
              <p className="text-sm text-neutral-500 dark:text-neutral-400 max-w-sm mt-1">
                When other users invite you to their maps, they will appear here.
              </p>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
              {sharedProjects.map(({ project, owner_name, owner_email, role, shared_date }) => (
                <div
                  key={project.id}
                  onClick={() => navigate(`/workspace?project=${encodeURIComponent(project.id)}`)}
                  className="group relative bg-white dark:bg-[#202020] hover:bg-neutral-50 dark:hover:bg-[#242424] border border-neutral-200 dark:border-[#2e2e2e] hover:border-[#3ecf8e] dark:hover:border-[#3ecf8e] rounded-[12px] p-5 transition-all duration-200 cursor-pointer flex flex-col justify-between shadow-xs hover:shadow-md"
                >
                  <div>
                    <div className="flex items-start justify-between gap-3 mb-2">
                      <div className="flex items-center gap-2">
                        <span className={`text-[10px] px-1.5 py-0.5 rounded-[4px] font-semibold uppercase ${
                          role === "editor"
                            ? "bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30"
                            : "bg-purple-500/10 text-purple-600 dark:text-purple-400 border border-purple-500/20"
                        }`}>
                          {role}
                        </span>
                        <h3 className="text-base font-semibold text-neutral-900 dark:text-white group-hover:text-[#24b47e] dark:group-hover:text-[#3ecf8e] transition-colors line-clamp-1">
                          {project.name}
                        </h3>
                      </div>
                    </div>

                    <p className="text-xs text-neutral-500 dark:text-neutral-400 line-clamp-2 min-h-[32px]">
                      {project.description || "No description provided."}
                    </p>

                    <div className="mt-2 text-[11px] text-neutral-500 dark:text-neutral-400">
                      Owner: <span className="text-neutral-700 dark:text-neutral-300 font-medium">{owner_name}</span> ({owner_email})
                    </div>
                  </div>

                  <div className="mt-6 pt-4 border-t border-neutral-100 dark:border-[#2e2e2e] flex items-center justify-between text-[11px] text-neutral-500 dark:text-neutral-400">
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
        ) : activeTab === "organizations" ? (
          /* ORGANIZATIONS GRID */
          organizations.length === 0 ? (
            <div className="border border-dashed border-neutral-300 dark:border-[#2e2e2e] rounded-[12px] p-12 text-center flex flex-col items-center justify-center">
              <div className="w-12 h-12 rounded-[8px] bg-neutral-100 dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-center text-neutral-400 mb-4">
                <svg className="w-6 h-6 text-[#24b47e] dark:text-[#3ecf8e]" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" />
                </svg>
              </div>
              <h3 className="text-base font-semibold text-neutral-900 dark:text-white">No organizations yet</h3>
              <p className="text-sm text-neutral-500 dark:text-neutral-400 max-w-sm mt-1 mb-6">
                Create an organization workspace to manage team members, assign roles, and share datasets.
              </p>
              <button
                onClick={() => setShowCreateOrgModal(true)}
                className="px-4 py-2 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-medium text-sm transition-colors cursor-pointer shadow-xs"
              >
                Create New Organization
              </button>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
              {organizations.map((org) => (
                <div
                  key={org.id}
                  onClick={() => navigate(`/organizations/${encodeURIComponent(org.id)}`)}
                  className="group relative bg-white dark:bg-[#202020] hover:bg-neutral-50 dark:hover:bg-[#242424] border border-neutral-200 dark:border-[#2e2e2e] hover:border-[#3ecf8e] dark:hover:border-[#3ecf8e] rounded-[12px] p-5 transition-all duration-200 cursor-pointer flex flex-col justify-between shadow-xs hover:shadow-md"
                >
                  <div>
                    <div className="flex items-start justify-between gap-3 mb-2">
                      <div className="flex items-center gap-2">
                        <span className={`text-[10px] px-1.5 py-0.5 rounded-[4px] font-semibold uppercase ${
                          org.user_role === "owner"
                            ? "bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30"
                            : org.user_role === "admin"
                            ? "bg-purple-500/10 text-purple-600 dark:text-purple-400 border border-purple-500/20"
                            : "bg-neutral-500/10 text-neutral-600 dark:text-neutral-400 border border-neutral-500/20"
                        }`}>
                          {org.user_role || "member"}
                        </span>
                        <h3 className="text-base font-semibold text-neutral-900 dark:text-white group-hover:text-[#24b47e] dark:group-hover:text-[#3ecf8e] transition-colors line-clamp-1">
                          {org.name}
                        </h3>
                      </div>
                    </div>

                    <p className="text-xs text-neutral-500 dark:text-neutral-400 line-clamp-2 min-h-[32px]">
                      {org.description || "No description provided."}
                    </p>
                  </div>

                  <div className="mt-6 pt-4 border-t border-neutral-100 dark:border-[#2e2e2e] flex items-center justify-between text-[11px] text-neutral-500 dark:text-neutral-400">
                    <span>
                      {org.member_count || 1} {org.member_count === 1 ? "member" : "members"}
                    </span>
                    <span>
                      Created {new Date(org.created_at).toLocaleDateString()}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          )
        ) : activeTab === "invitations" ? (
          /* INVITATIONS LIST */
          invitations.length === 0 ? (
            <div className="border border-dashed border-neutral-300 dark:border-[#2e2e2e] rounded-[12px] p-12 text-center flex flex-col items-center justify-center">
              <h3 className="text-base font-semibold text-neutral-900 dark:text-white">No pending invitations</h3>
              <p className="text-sm text-neutral-500 dark:text-neutral-400 max-w-sm mt-1">
                You're all caught up! New invites from team members will appear here.
              </p>
            </div>
          ) : (
            <div className="space-y-3 max-w-3xl">
              {invitations.map((inv) => (
                <div
                  key={inv.invitation_id || inv.id}
                  className="p-4 rounded-[12px] bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-between gap-4 shadow-xs"
                >
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="text-xs font-semibold px-2 py-0.5 rounded-[4px] bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30 uppercase">
                        {inv.inviteType || "project"} : {inv.role}
                      </span>
                      <h4 className="text-sm font-bold text-neutral-900 dark:text-white">{inv.project_name || inv.org_name}</h4>
                    </div>
                    <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-1">
                      Invited by <span className="text-neutral-700 dark:text-neutral-200 font-medium">{inv.inviter_name || inv.invited_by}</span> on {new Date(inv.created_at).toLocaleDateString()}
                    </p>
                  </div>

                  <div className="flex items-center gap-2 shrink-0">
                    <button
                      onClick={() => handleRespondInvite(inv, true)}
                      className="px-3.5 py-1.5 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] text-xs font-medium transition-colors cursor-pointer shadow-xs"
                    >
                      Accept
                    </button>
                    <button
                      onClick={() => handleRespondInvite(inv, false)}
                      className="px-3.5 py-1.5 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] hover:bg-neutral-100 dark:hover:bg-[#282828] text-neutral-700 dark:text-neutral-300 text-xs font-medium transition-colors cursor-pointer"
                    >
                      Decline
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )
        ) : activeTab === "activity" ? (
          /* RECENT ACTIVITY TIMELINE */
          activities.length === 0 ? (
            <div className="border border-dashed border-neutral-300 dark:border-[#2e2e2e] rounded-[12px] p-12 text-center flex flex-col items-center justify-center">
              <div className="w-12 h-12 rounded-[8px] bg-neutral-100 dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-center text-neutral-400 mb-4">
                <svg className="w-6 h-6 text-neutral-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
              </div>
              <h3 className="text-base font-semibold text-neutral-900 dark:text-white">No recent activity</h3>
              <p className="text-sm text-neutral-500 dark:text-neutral-400 max-w-sm mt-1">
                Recent actions across your projects and organizations will appear here.
              </p>
            </div>
          ) : (
            <div className="space-y-3 max-w-4xl">
              {activities.map((act, idx) => (
                <div
                  key={act.id || idx}
                  className="p-4 rounded-[12px] bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] flex items-start gap-4 shadow-xs"
                >
                  <div className="w-8 h-8 rounded-[6px] bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30 flex items-center justify-center shrink-0 mt-0.5">
                    <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                    </svg>
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 flex-wrap">
                      <span className="text-xs font-bold text-neutral-900 dark:text-white">{act.user_name || "User"}</span>
                      <span className="text-xs text-neutral-600 dark:text-neutral-400">{act.details || act.action}</span>
                      {act.source && (
                        <span className="text-[10px] px-2 py-0.5 rounded-[4px] bg-neutral-100 dark:bg-[#171717] text-neutral-600 dark:text-neutral-300 border border-neutral-200 dark:border-[#2e2e2e]">
                          {act.source}
                        </span>
                      )}
                    </div>
                    <span className="text-[11px] text-neutral-400 mt-1 block">
                      {new Date(act.created_at).toLocaleString()}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          )
        ) : (
          /* ACCOUNT SETTINGS VIEW */
          <div className="max-w-3xl space-y-6">
            <div className="p-6 rounded-[12px] bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] shadow-xs">
              <h3 className="text-base font-bold text-neutral-900 dark:text-white mb-4 flex items-center gap-2">
                <svg className="w-5 h-5 text-[#24b47e] dark:text-[#3ecf8e]" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
                </svg>
                User Profile
              </h3>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 text-sm">
                <div>
                  <span className="text-xs text-neutral-500 dark:text-neutral-400 block mb-1">Full Name</span>
                  <div className="p-2.5 rounded-[6px] bg-neutral-50 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] text-neutral-900 dark:text-white font-medium">
                    {user?.name || "Not provided"}
                  </div>
                </div>
                <div>
                  <span className="text-xs text-neutral-500 dark:text-neutral-400 block mb-1">Email Address</span>
                  <div className="p-2.5 rounded-[6px] bg-neutral-50 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] text-neutral-900 dark:text-white font-medium">
                    {user?.email || "Not provided"}
                  </div>
                </div>
                <div>
                  <span className="text-xs text-neutral-500 dark:text-neutral-400 block mb-1">User Identifier</span>
                  <div className="p-2.5 rounded-[6px] bg-neutral-50 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] text-neutral-600 dark:text-neutral-300 font-mono text-xs">
                    {user?.id || "N/A"}
                  </div>
                </div>
                <div>
                  <span className="text-xs text-neutral-500 dark:text-neutral-400 block mb-1">Account Status</span>
                  <div className="p-2.5 rounded-[6px] bg-neutral-50 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] text-[#24b47e] dark:text-[#3ecf8e] font-medium flex items-center gap-1.5">
                    <span className="w-2 h-2 rounded-full bg-[#3ecf8e]"></span>
                    Authenticated & Active
                  </div>
                </div>
              </div>
            </div>

            <div className="p-6 rounded-[12px] bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] shadow-xs">
              <h3 className="text-base font-bold text-neutral-900 dark:text-white mb-4 flex items-center gap-2">
                <svg className="w-5 h-5 text-[#24b47e] dark:text-[#3ecf8e]" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 7v10c0 2 1.5 3 3.5 3h9c2 0 3.5-1 3.5-3V7c0-2-1.5-3-3.5-3h-9C5.5 4 4 5 4 7z" />
                </svg>
                Database & Security
              </h3>
              <div className="space-y-3 text-sm">
                <div className="flex items-center justify-between p-3 rounded-[6px] bg-neutral-50 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e]">
                  <div>
                    <span className="text-neutral-900 dark:text-white font-medium block">Database Engine</span>
                    <span className="text-xs text-neutral-500 dark:text-neutral-400">Microsoft SQL Server 2022 (Port 1434 / veracity_maps)</span>
                  </div>
                  <span className="text-xs px-2 py-0.5 rounded-[4px] bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30 font-semibold">
                    CONNECTED
                  </span>
                </div>
                <div className="flex items-center justify-between p-3 rounded-[6px] bg-neutral-50 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e]">
                  <div>
                    <span className="text-neutral-900 dark:text-white font-medium block">Password Cryptography</span>
                    <span className="text-xs text-neutral-500 dark:text-neutral-400">SHA-256 with unique 16-byte cryptographic salt per user</span>
                  </div>
                  <span className="text-xs px-2 py-0.5 rounded-[4px] bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30 font-semibold">
                    ACTIVE
                  </span>
                </div>
                <div className="flex items-center justify-between p-3 rounded-[6px] bg-neutral-50 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e]">
                  <div>
                    <span className="text-neutral-900 dark:text-white font-medium block">Token Storage & Revocation</span>
                    <span className="text-xs text-neutral-500 dark:text-neutral-400">Persisted in MSSQL AuthTokens table; immediate revocation on logout</span>
                  </div>
                  <span className="text-xs px-2 py-0.5 rounded-[4px] bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30 font-semibold">
                    SECURED
                  </span>
                </div>
              </div>

              <div className="mt-6 pt-4 border-t border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-between">
                <span className="text-xs text-neutral-500 dark:text-neutral-400">Appearance Mode</span>
                <ThemeToggle showLabel={true} />
              </div>
            </div>
          </div>
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
          <div className="bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] rounded-[12px] max-w-md w-full p-6 shadow-2xl">
            <h2 className="text-lg font-bold text-neutral-900 dark:text-white mb-1">Create New Project</h2>
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mb-4">Set up a private workspace container for your maps.</p>

            <form onSubmit={handleCreate} className="space-y-4">
              <div>
                <label className="block text-xs font-medium text-neutral-700 dark:text-neutral-300 uppercase tracking-wider mb-1.5">
                  Project Name
                </label>
                <input
                  type="text"
                  required
                  autoFocus
                  value={newProjectName}
                  onChange={(e) => setNewProjectName(e.target.value)}
                  placeholder="e.g. Downtown Facility Revitalization"
                  className="w-full px-3 py-2 rounded-[6px] bg-neutral-50 dark:bg-[#171717] border border-neutral-300 dark:border-[#2e2e2e] text-neutral-900 dark:text-white text-sm focus:outline-none focus:border-[#3ecf8e]"
                />
              </div>

              <div>
                <label className="block text-xs font-medium text-neutral-700 dark:text-neutral-300 uppercase tracking-wider mb-1.5">
                  Description (Optional)
                </label>
                <textarea
                  rows={3}
                  value={newProjectDesc}
                  onChange={(e) => setNewProjectDesc(e.target.value)}
                  placeholder="Notes, geographic location, or project scope..."
                  className="w-full px-3 py-2 rounded-[6px] bg-neutral-50 dark:bg-[#171717] border border-neutral-300 dark:border-[#2e2e2e] text-neutral-900 dark:text-white text-sm focus:outline-none focus:border-[#3ecf8e] resize-none"
                />
              </div>

              <div className="flex items-center justify-end gap-3 pt-2">
                <button
                  type="button"
                  onClick={() => setShowCreateModal(false)}
                  className="px-4 py-2 rounded-[6px] text-sm text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#282828] transition-colors cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={creating || !newProjectName.trim()}
                  className="px-4 py-2 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] text-sm font-medium transition-colors disabled:opacity-50 cursor-pointer shadow-xs"
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
          <div className="bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] rounded-[12px] max-w-md w-full p-6 shadow-2xl">
            <h2 className="text-lg font-bold text-neutral-900 dark:text-white mb-1">Edit Project</h2>
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mb-4">Update the name and description of this project.</p>

            <form onSubmit={handleUpdate} className="space-y-4">
              <div>
                <label className="block text-xs font-medium text-neutral-700 dark:text-neutral-300 uppercase tracking-wider mb-1.5">
                  Project Name
                </label>
                <input
                  type="text"
                  required
                  autoFocus
                  value={editName}
                  onChange={(e) => setEditName(e.target.value)}
                  className="w-full px-3 py-2 rounded-[6px] bg-neutral-50 dark:bg-[#171717] border border-neutral-300 dark:border-[#2e2e2e] text-neutral-900 dark:text-white text-sm focus:outline-none focus:border-[#3ecf8e]"
                />
              </div>

              <div>
                <label className="block text-xs font-medium text-neutral-700 dark:text-neutral-300 uppercase tracking-wider mb-1.5">
                  Description
                </label>
                <textarea
                  rows={3}
                  value={editDesc}
                  onChange={(e) => setEditDesc(e.target.value)}
                  className="w-full px-3 py-2 rounded-[6px] bg-neutral-50 dark:bg-[#171717] border border-neutral-300 dark:border-[#2e2e2e] text-neutral-900 dark:text-white text-sm focus:outline-none focus:border-[#3ecf8e] resize-none"
                />
              </div>

              <div className="flex items-center justify-end gap-3 pt-2">
                <button
                  type="button"
                  onClick={() => setEditingProject(null)}
                  className="px-4 py-2 rounded-[6px] text-sm text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#282828] transition-colors cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={updating || !editName.trim()}
                  className="px-4 py-2 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] text-sm font-medium transition-colors disabled:opacity-50 cursor-pointer shadow-xs"
                >
                  {updating ? "Saving..." : "Save Changes"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* New Organization Modal */}
      {showCreateOrgModal && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] rounded-[12px] max-w-md w-full p-6 shadow-2xl">
            <h2 className="text-lg font-bold text-neutral-900 dark:text-white mb-1">Create New Organization</h2>
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mb-4">Set up an organization workspace for team collaboration.</p>

            <form onSubmit={handleCreateOrg} className="space-y-4">
              <div>
                <label className="block text-xs font-medium text-neutral-700 dark:text-neutral-300 uppercase tracking-wider mb-1.5">
                  Organization Name
                </label>
                <input
                  type="text"
                  required
                  autoFocus
                  placeholder="e.g. Acme Mapping Team"
                  value={newOrgName}
                  onChange={(e) => setNewOrgName(e.target.value)}
                  className="w-full px-3 py-2 rounded-[6px] bg-neutral-50 dark:bg-[#171717] border border-neutral-300 dark:border-[#2e2e2e] text-neutral-900 dark:text-white text-sm focus:outline-none focus:border-[#3ecf8e]"
                />
              </div>

              <div>
                <label className="block text-xs font-medium text-neutral-700 dark:text-neutral-300 uppercase tracking-wider mb-1.5">
                  Description
                </label>
                <textarea
                  rows={3}
                  placeholder="Describe your organization's projects or purpose..."
                  value={newOrgDesc}
                  onChange={(e) => setNewOrgDesc(e.target.value)}
                  className="w-full px-3 py-2 rounded-[6px] bg-neutral-50 dark:bg-[#171717] border border-neutral-300 dark:border-[#2e2e2e] text-neutral-900 dark:text-white text-sm focus:outline-none focus:border-[#3ecf8e] resize-none"
                />
              </div>

              <div className="flex items-center justify-end gap-3 pt-2">
                <button
                  type="button"
                  onClick={() => setShowCreateOrgModal(false)}
                  className="px-4 py-2 rounded-[6px] text-sm text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#282828] transition-colors cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={creatingOrg || !newOrgName.trim()}
                  className="px-4 py-2 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] text-sm font-medium transition-colors disabled:opacity-50 cursor-pointer shadow-xs"
                >
                  {creatingOrg ? "Creating..." : "Create Organization"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}
