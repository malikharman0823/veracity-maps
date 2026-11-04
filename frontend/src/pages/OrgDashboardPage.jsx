import React, { useState, useEffect } from "react"
import { useParams, useNavigate, Link } from "react-router-dom"
import { useAuth } from "../context/AuthContext"
import {
  getOrganization,
  updateOrganization,
  deleteOrganization,
  listOrgMembers,
  inviteOrgMember,
  updateOrgMemberRole,
  removeOrgMember,
  listOrgProjects,
  createOrgProject,
  transferProjectToOrg,
  listOrgActivities,
  listUserProjects,
} from "../api"

export default function OrgDashboardPage() {
  const { id: orgId } = useParams()
  const { user, logout } = useAuth()
  const navigate = useNavigate()

  const [activeTab, setActiveTab] = useState("overview") // "overview" | "projects" | "members" | "activity" | "settings"
  const [org, setOrg] = useState(null)
  const [userRole, setUserRole] = useState("viewer")
  const [members, setMembers] = useState([])
  const [projects, setProjects] = useState([])
  const [activities, setActivities] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")

  // Invite modal state
  const [showInviteModal, setShowInviteModal] = useState(false)
  const [inviteEmail, setInviteEmail] = useState("")
  const [inviteRole, setInviteRole] = useState("viewer")
  const [inviting, setInviting] = useState(false)

  // Create project modal state
  const [showCreateProjModal, setShowCreateProjModal] = useState(false)
  const [newProjName, setNewProjName] = useState("")
  const [newProjDesc, setNewProjDesc] = useState("")
  const [creatingProj, setCreatingProj] = useState(false)

  // Transfer project modal state
  const [showTransferModal, setShowTransferModal] = useState(false)
  const [myPersonalProjects, setMyPersonalProjects] = useState([])
  const [selectedPersonalProjId, setSelectedPersonalProjId] = useState("")
  const [transferring, setTransferring] = useState(false)

  // Settings update state
  const [editOrgName, setEditOrgName] = useState("")
  const [editOrgDesc, setEditOrgDesc] = useState("")
  const [savingSettings, setSavingSettings] = useState(false)

  const isOwner = userRole === "owner"
  const isAdmin = isOwner || userRole === "admin"
  const isEditor = isAdmin || userRole === "editor"

  const fetchOrgDetails = async () => {
    try {
      setLoading(true)
      setError("")
      const orgData = await getOrganization(orgId)
      setOrg(orgData.organization)
      setUserRole(orgData.user_role)
      setEditOrgName(orgData.organization.name)
      setEditOrgDesc(orgData.organization.description || "")

      const [mList, pList] = await Promise.all([
        listOrgMembers(orgId).catch(() => []),
        listOrgProjects(orgId).catch(() => []),
      ])
      setMembers(mList)
      setProjects(pList)

      if (orgData.user_role === "owner" || orgData.user_role === "admin") {
        const actList = await listOrgActivities(orgId).catch(() => [])
        setActivities(actList)
      }
    } catch (err) {
      setError(err.response?.data?.error?.message || "Failed to load organization")
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchOrgDetails()
  }, [orgId])

  const handleInvite = async (e) => {
    e.preventDefault()
    if (!inviteEmail.trim()) return

    setInviting(true)
    try {
      await inviteOrgMember(orgId, inviteEmail.trim(), inviteRole)
      setInviteEmail("")
      setShowInviteModal(false)
      const updatedMembers = await listOrgMembers(orgId)
      setMembers(updatedMembers)
      alert("Invitation sent successfully.")
    } catch (err) {
      alert("Failed to invite member: " + (err.response?.data?.error?.message || err.message))
    } finally {
      setInviting(false)
    }
  }

  const handleRoleChange = async (memberId, newRole) => {
    try {
      await updateOrgMemberRole(orgId, memberId, newRole)
      setMembers((prev) =>
        prev.map((m) => (m.id === memberId ? { ...m, role: newRole } : m))
      )
    } catch (err) {
      alert("Failed to change role: " + (err.response?.data?.error?.message || err.message))
    }
  }

  const handleRemoveMember = async (member) => {
    if (!window.confirm(`Are you sure you want to remove ${member.user_name || member.user_email} from the organization?`)) {
      return
    }

    try {
      await removeOrgMember(orgId, member.id)
      setMembers((prev) => prev.filter((m) => m.id !== member.id))
    } catch (err) {
      alert("Failed to remove member: " + (err.response?.data?.error?.message || err.message))
    }
  }

  const handleCreateProject = async (e) => {
    e.preventDefault()
    if (!newProjName.trim()) return

    setCreatingProj(true)
    try {
      const p = await createOrgProject(orgId, newProjName.trim(), newProjDesc.trim())
      setShowCreateProjModal(false)
      setNewProjName("")
      setNewProjDesc("")
      const updatedProjects = await listOrgProjects(orgId)
      setProjects(updatedProjects)
      navigate(`/workspace?project=${encodeURIComponent(p.id)}`)
    } catch (err) {
      alert("Failed to create project: " + (err.response?.data?.error?.message || err.message))
    } finally {
      setCreatingProj(false)
    }
  }

  const handleOpenTransferModal = async () => {
    try {
      const owned = await listUserProjects()
      const personalOnly = owned.filter((p) => !p.org_id)
      setMyPersonalProjects(personalOnly)
      if (personalOnly.length > 0) {
        setSelectedPersonalProjId(personalOnly[0].id)
      }
      setShowTransferModal(true)
    } catch (err) {
      alert("Could not load personal projects")
    }
  }

  const handleTransferProject = async (e) => {
    e.preventDefault()
    if (!selectedPersonalProjId) return

    setTransferring(true)
    try {
      await transferProjectToOrg(orgId, selectedPersonalProjId)
      setShowTransferModal(false)
      const updatedProjects = await listOrgProjects(orgId)
      setProjects(updatedProjects)
      alert("Project transferred to organization successfully.")
    } catch (err) {
      alert("Transfer failed: " + (err.response?.data?.error?.message || err.message))
    } finally {
      setTransferring(false)
    }
  }

  const handleSaveSettings = async (e) => {
    e.preventDefault()
    setSavingSettings(true)
    try {
      const updated = await updateOrganization(orgId, {
        name: editOrgName.trim(),
        description: editOrgDesc.trim(),
      })
      setOrg(updated)
      alert("Organization settings updated.")
    } catch (err) {
      alert("Failed to update settings: " + (err.response?.data?.error?.message || err.message))
    } finally {
      setSavingSettings(false)
    }
  }

  const handleDeleteOrg = async () => {
    if (!window.confirm(`DANGER: Are you sure you want to delete organization "${org.name}"? All organizational projects and files will be permanently deleted.`)) {
      return
    }

    try {
      await deleteOrganization(orgId)
      navigate("/dashboard")
    } catch (err) {
      alert("Failed to delete organization: " + (err.response?.data?.error?.message || err.message))
    }
  }

  const formatBytes = (bytes) => {
    if (!bytes || bytes === 0) return "0 MB"
    const mb = bytes / (1024 * 1024)
    if (mb >= 1024) return (mb / 1024).toFixed(2) + " GB"
    return mb.toFixed(1) + " MB"
  }

  if (loading) {
    return (
      <div className="min-h-screen bg-slate-950 flex items-center justify-center text-slate-400 text-sm">
        <svg className="animate-spin h-6 w-6 text-blue-500 mr-2" fill="none" viewBox="0 0 24 24">
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
          <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
        </svg>
        Loading organization workspace...
      </div>
    )
  }

  if (error || !org) {
    return (
      <div className="min-h-screen bg-slate-950 flex flex-col items-center justify-center p-6 text-center">
        <div className="w-12 h-12 rounded-xl bg-red-500/10 border border-red-500/20 text-red-400 flex items-center justify-center mb-4">
          <svg className="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
          </svg>
        </div>
        <h2 className="text-xl font-bold text-white mb-2">Access Restricted</h2>
        <p className="text-sm text-slate-400 mb-6 max-w-md">{error || "Organization not found."}</p>
        <Link
          to="/dashboard"
          className="inline-flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500 transition-colors"
        >
          ← Back to Dashboard
        </Link>
      </div>
    )
  }

  const quotaPercent = Math.min(
    100,
    Math.round(((org.storage_used_bytes || 0) / (org.storage_quota_bytes || 10737418240)) * 100)
  )

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100 flex flex-col">
      {/* Top Header */}
      <header className="h-16 border-b border-slate-800 bg-slate-900/50 backdrop-blur-md px-6 flex items-center justify-between sticky top-0 z-30">
        <div className="flex items-center gap-4">
          <Link
            to="/dashboard"
            className="p-1.5 rounded-lg bg-slate-800/80 hover:bg-slate-700 text-slate-300 hover:text-white border border-slate-700/80 transition-colors"
            title="Back to Dashboard"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 19l-7-7m0 0l7-7m-7 7h18" />
            </svg>
          </Link>
          <div className="w-8 h-8 rounded-lg bg-blue-600/20 border border-blue-500/30 flex items-center justify-center text-blue-400 font-bold">
            {org.name?.charAt(0) || "O"}
          </div>
          <div>
            <div className="flex items-center gap-2">
              <span className="font-semibold text-lg text-white tracking-tight">{org.name}</span>
              <span className="text-[10px] px-2 py-0.5 rounded-full bg-blue-500/10 text-blue-400 border border-blue-500/20 font-semibold uppercase">
                {userRole}
              </span>
            </div>
            <p className="text-xs text-slate-400 line-clamp-1">{org.description || "Organization Workspace"}</p>
          </div>
        </div>

        <div className="flex items-center gap-3">
          {isEditor && (
            <button
              type="button"
              onClick={() => setShowCreateProjModal(true)}
              className="inline-flex items-center gap-2 px-3.5 py-2 rounded-lg bg-blue-600 hover:bg-blue-500 text-white font-medium text-xs transition-colors shadow-sm cursor-pointer"
            >
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
              </svg>
              New Org Project
            </button>
          )}
          {isAdmin && (
            <button
              type="button"
              onClick={handleOpenTransferModal}
              className="inline-flex items-center gap-2 px-3 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 text-xs font-medium transition-colors cursor-pointer"
            >
              <svg className="w-3.5 h-3.5 text-slate-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4" />
              </svg>
              Transfer Project
            </button>
          )}
          <button
            onClick={logout}
            className="px-3 py-1.5 rounded-lg text-xs font-medium bg-slate-800 hover:bg-slate-700 text-slate-300 hover:text-white border border-slate-700 transition-colors"
          >
            Log out
          </button>
        </div>
      </header>

      {/* Main Content Area */}
      <main className="flex-1 max-w-7xl w-full mx-auto px-6 py-6">
        {/* Section Tabs */}
        <div className="flex items-center gap-2 border-b border-slate-800 pb-3 mb-6">
          <button
            onClick={() => setActiveTab("overview")}
            className={`px-4 py-2 rounded-lg text-sm font-medium transition-all ${
              activeTab === "overview"
                ? "bg-blue-600 text-white shadow-sm"
                : "text-slate-400 hover:text-white hover:bg-slate-900"
            }`}
          >
            Overview
          </button>
          <button
            onClick={() => setActiveTab("projects")}
            className={`px-4 py-2 rounded-lg text-sm font-medium transition-all ${
              activeTab === "projects"
                ? "bg-blue-600 text-white shadow-sm"
                : "text-slate-400 hover:text-white hover:bg-slate-900"
            }`}
          >
            Projects ({projects.length})
          </button>
          <button
            onClick={() => setActiveTab("members")}
            className={`px-4 py-2 rounded-lg text-sm font-medium transition-all ${
              activeTab === "members"
                ? "bg-blue-600 text-white shadow-sm"
                : "text-slate-400 hover:text-white hover:bg-slate-900"
            }`}
          >
            Members ({members.length})
          </button>
          {isAdmin && (
            <button
              onClick={() => setActiveTab("activity")}
              className={`px-4 py-2 rounded-lg text-sm font-medium transition-all ${
                activeTab === "activity"
                  ? "bg-blue-600 text-white shadow-sm"
                  : "text-slate-400 hover:text-white hover:bg-slate-900"
              }`}
            >
              Activity Logs
            </button>
          )}
          {isAdmin && (
            <button
              onClick={() => setActiveTab("settings")}
              className={`px-4 py-2 rounded-lg text-sm font-medium transition-all ${
                activeTab === "settings"
                  ? "bg-blue-600 text-white shadow-sm"
                  : "text-slate-400 hover:text-white hover:bg-slate-900"
              }`}
            >
              Settings
            </button>
          )}
        </div>

        {/* TAB 1: OVERVIEW */}
        {activeTab === "overview" && (
          <div className="space-y-6">
            {/* Metric Cards */}
            <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
              <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-5 shadow-sm">
                <div className="flex items-center justify-between text-slate-400 text-xs font-semibold uppercase tracking-wider">
                  <span>Total Projects</span>
                  <span className="text-blue-400 font-bold">#</span>
                </div>
                <div className="mt-3 text-3xl font-extrabold text-white">{projects.length}</div>
                <p className="mt-1 text-xs text-slate-400">Collaborative map workspaces</p>
              </div>

              <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-5 shadow-sm">
                <div className="flex items-center justify-between text-slate-400 text-xs font-semibold uppercase tracking-wider">
                  <span>Total Members</span>
                  <span className="text-emerald-400 font-bold">●</span>
                </div>
                <div className="mt-3 text-3xl font-extrabold text-white">{members.length}</div>
                <p className="mt-1 text-xs text-slate-400">Active teammates & collaborators</p>
              </div>

              <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-5 shadow-sm">
                <div className="flex items-center justify-between text-slate-400 text-xs font-semibold uppercase tracking-wider">
                  <span>Storage Used</span>
                  <span className="text-amber-400 font-bold">MB</span>
                </div>
                <div className="mt-3 text-2xl font-extrabold text-white">
                  {formatBytes(org.storage_used_bytes)}
                </div>
                <div className="mt-2 w-full bg-slate-800 rounded-full h-1.5 overflow-hidden">
                  <div
                    className="bg-blue-500 h-1.5 rounded-full"
                    style={{ width: `${quotaPercent}%` }}
                  />
                </div>
                <p className="mt-1 text-[10px] text-slate-400 text-right">
                  {quotaPercent}% of {formatBytes(org.storage_quota_bytes || 10737418240)}
                </p>
              </div>

              <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-5 shadow-sm">
                <div className="flex items-center justify-between text-slate-400 text-xs font-semibold uppercase tracking-wider">
                  <span>Your Role</span>
                  <span className="text-purple-400 font-bold">★</span>
                </div>
                <div className="mt-3 text-2xl font-extrabold text-white capitalize">{userRole}</div>
                <p className="mt-1 text-xs text-slate-400">
                  {isOwner ? "Complete ownership" : isAdmin ? "Team administration" : isEditor ? "Full editing access" : "Read-only inspection"}
                </p>
              </div>
            </div>

            {/* Quick Actions & Recent Projects */}
            <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
              <div className="md:col-span-2 rounded-xl border border-slate-800 bg-slate-900/60 p-6 shadow-sm">
                <div className="flex items-center justify-between mb-4">
                  <h3 className="text-base font-semibold text-white">Organization Projects</h3>
                  <button
                    onClick={() => setActiveTab("projects")}
                    className="text-xs font-semibold text-blue-400 hover:text-blue-300"
                  >
                    View All →
                  </button>
                </div>
                {projects.length === 0 ? (
                  <div className="py-8 text-center text-xs text-slate-400">
                    No projects yet. Create your organization's first mapping project!
                  </div>
                ) : (
                  <div className="space-y-3">
                    {projects.slice(0, 4).map((p) => (
                      <div
                        key={p.id}
                        onClick={() => navigate(`/workspace?project=${encodeURIComponent(p.id)}`)}
                        className="flex items-center justify-between p-3.5 rounded-xl border border-slate-800/80 bg-slate-950/40 hover:bg-slate-900 hover:border-blue-500/40 transition-all cursor-pointer"
                      >
                        <div className="flex items-center gap-3">
                          <div className="w-8 h-8 rounded-lg bg-blue-600/20 text-blue-400 flex items-center justify-center font-bold text-xs">
                            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l4.553 2.276A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7" />
                            </svg>
                          </div>
                          <div>
                            <div className="text-sm font-semibold text-white">{p.name}</div>
                            <div className="text-xs text-slate-400 line-clamp-1">{p.description || "No description"}</div>
                          </div>
                        </div>
                        <div className="text-[11px] text-slate-400">
                          Updated {new Date(p.updated_at).toLocaleDateString()}
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-6 shadow-sm">
                <h3 className="text-base font-semibold text-white mb-4">Workspace Info</h3>
                <div className="space-y-3.5 text-xs">
                  <div>
                    <span className="text-slate-400 block text-[11px] uppercase tracking-wider mb-0.5">Organization ID</span>
                    <span className="font-mono text-slate-200 select-all bg-slate-950 px-2 py-1 rounded border border-slate-800 block text-[11px] truncate">{org.id}</span>
                  </div>
                  <div>
                    <span className="text-slate-400 block text-[11px] uppercase tracking-wider mb-0.5">Created Date</span>
                    <span className="text-slate-200">{new Date(org.created_at).toLocaleDateString()}</span>
                  </div>
                  <div>
                    <span className="text-slate-400 block text-[11px] uppercase tracking-wider mb-0.5">Isolation Level</span>
                    <span className="text-emerald-400 font-medium">Independent Workspace</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        )}

        {/* TAB 2: PROJECTS */}
        {activeTab === "projects" && (
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <h2 className="text-lg font-bold text-white">Projects</h2>
                <p className="text-xs text-slate-400">Shared geospatial layers, floors, and vector markers.</p>
              </div>
              {isEditor && (
                <button
                  type="button"
                  onClick={() => setShowCreateProjModal(true)}
                  className="inline-flex items-center gap-2 px-3.5 py-2 rounded-lg bg-blue-600 hover:bg-blue-500 text-white font-medium text-xs transition-colors shadow-sm cursor-pointer"
                >
                  <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                  </svg>
                  New Org Project
                </button>
              )}
            </div>

            {projects.length === 0 ? (
              <div className="rounded-2xl border border-dashed border-slate-800 bg-slate-900/40 p-12 text-center">
                <h3 className="text-base font-semibold text-white mb-1">No Projects Found</h3>
                <p className="text-xs text-slate-400 mb-4">
                  This organization doesn't have any projects yet.
                </p>
                {isEditor && (
                  <button
                    onClick={() => setShowCreateProjModal(true)}
                    className="inline-flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2 text-xs font-medium text-white hover:bg-blue-500 transition-colors"
                  >
                    Create Project
                  </button>
                )}
              </div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
                {projects.map((p) => (
                  <div
                    key={p.id}
                    onClick={() => navigate(`/workspace?project=${encodeURIComponent(p.id)}`)}
                    className="group relative bg-slate-900/60 hover:bg-slate-900 border border-slate-800 hover:border-blue-500/50 rounded-xl p-5 transition-all duration-200 cursor-pointer flex flex-col justify-between shadow-sm hover:shadow-md"
                  >
                    <div>
                      <div className="flex items-start justify-between gap-3 mb-2">
                        <div className="flex items-center gap-2">
                          <span className="text-[10px] px-1.5 py-0.5 rounded bg-blue-500/10 text-blue-400 border border-blue-500/20 font-semibold uppercase">
                            Org Project
                          </span>
                          <h3 className="text-base font-semibold text-white group-hover:text-blue-400 transition-colors line-clamp-1">
                            {p.name}
                          </h3>
                        </div>
                      </div>
                      <p className="text-xs text-slate-400 line-clamp-2 min-h-[32px]">
                        {p.description || "No description provided."}
                      </p>
                    </div>

                    <div className="mt-6 pt-4 border-t border-slate-800/80 flex items-center justify-between text-[11px] text-slate-500">
                      <span>{p.dataset_ids?.length || 0} layers</span>
                      <span>Updated {new Date(p.updated_at).toLocaleDateString()}</span>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {/* TAB 3: MEMBERS */}
        {activeTab === "members" && (
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <h2 className="text-lg font-bold text-white">Team Members</h2>
                <p className="text-xs text-slate-400">Manage organizational roles and pending member invitations.</p>
              </div>
              {isAdmin && (
                <button
                  type="button"
                  onClick={() => setShowInviteModal(true)}
                  className="inline-flex items-center gap-2 px-3.5 py-2 rounded-lg bg-blue-600 hover:bg-blue-500 text-white font-medium text-xs transition-colors shadow-sm cursor-pointer"
                >
                  <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                  </svg>
                  Invite Member
                </button>
              )}
            </div>

            <div className="rounded-xl border border-slate-800 bg-slate-900/60 overflow-hidden shadow-sm">
              <table className="w-full text-left text-xs border-collapse">
                <thead>
                  <tr className="border-b border-slate-800 bg-slate-900/80 text-slate-400 font-semibold">
                    <th className="py-3 px-4">Member</th>
                    <th className="py-3 px-4">Email</th>
                    <th className="py-3 px-4">Status</th>
                    <th className="py-3 px-4">Role</th>
                    {isAdmin && <th className="py-3 px-4 text-right">Actions</th>}
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-800/60">
                  {members.map((m) => {
                    const isSelf = m.user_id === user?.id
                    const isMemberOwner = m.role === "owner"

                    return (
                      <tr key={m.id} className="hover:bg-slate-900/80">
                        <td className="py-3 px-4 font-medium text-white">
                          <div className="flex items-center gap-2.5">
                            <div className="w-7 h-7 rounded-full bg-blue-600/30 text-blue-300 font-bold flex items-center justify-center text-[11px] border border-blue-500/20">
                              {m.user_name?.charAt(0) || "U"}
                            </div>
                            <span>{m.user_name || "Invited User"} {isSelf && "(You)"}</span>
                          </div>
                        </td>
                        <td className="py-3 px-4 text-slate-400">{m.user_email}</td>
                        <td className="py-3 px-4">
                          <span
                            className={`inline-flex items-center gap-1 font-mono text-[10px] uppercase font-bold px-2 py-0.5 rounded-full ${
                              m.invitation_status === "accepted"
                                ? "bg-emerald-500/10 text-emerald-400 border border-emerald-500/20"
                                : "bg-amber-500/10 text-amber-400 border border-amber-500/20"
                            }`}
                          >
                            {m.invitation_status}
                          </span>
                        </td>
                        <td className="py-3 px-4">
                          {isAdmin && !isMemberOwner && !isSelf ? (
                            <select
                              value={m.role}
                              onChange={(e) => handleRoleChange(m.id, e.target.value)}
                              className="rounded-lg border border-slate-700 bg-slate-950 px-2 py-1 text-xs text-white focus:outline-none focus:border-blue-500"
                            >
                              <option value="admin">Admin</option>
                              <option value="editor">Editor</option>
                              <option value="viewer">Viewer</option>
                            </select>
                          ) : (
                            <span className="font-semibold capitalize text-white">{m.role}</span>
                          )}
                        </td>
                        {isAdmin && (
                          <td className="py-3 px-4 text-right">
                            {!isMemberOwner && !isSelf && (
                              <button
                                type="button"
                                onClick={() => handleRemoveMember(m)}
                                className="p-1 rounded text-slate-400 hover:text-red-400 transition-colors cursor-pointer"
                                title="Remove member"
                              >
                                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                                </svg>
                              </button>
                            )}
                          </td>
                        )}
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {/* TAB 4: ACTIVITY LOG */}
        {activeTab === "activity" && isAdmin && (
          <div className="space-y-4">
            <h2 className="text-lg font-bold text-white">Audit & Activity Logs</h2>
            <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-5 shadow-sm">
              {activities.length === 0 ? (
                <div className="py-8 text-center text-xs text-slate-400">No activities logged yet.</div>
              ) : (
                <div className="space-y-3">
                  {activities.map((a) => (
                    <div key={a.id} className="flex items-start gap-3 text-xs border-b border-slate-800/60 pb-3 last:border-b-0">
                      <div className="w-2 h-2 rounded-full bg-blue-500 mt-1.5 shrink-0" />
                      <div className="flex-1">
                        <div className="font-semibold text-white">{a.user_name}</div>
                        <div className="text-slate-400 text-xs mt-0.5">{a.details}</div>
                      </div>
                      <div className="text-[10px] text-slate-400 font-mono shrink-0">
                        {new Date(a.timestamp).toLocaleString()}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        )}

        {/* TAB 5: SETTINGS */}
        {activeTab === "settings" && isAdmin && (
          <div className="max-w-2xl space-y-6">
            <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-6 shadow-sm">
              <h2 className="text-base font-bold text-white mb-4">Organization Settings</h2>
              <form onSubmit={handleSaveSettings} className="space-y-4">
                <div>
                  <label className="block text-xs font-semibold text-slate-300 uppercase tracking-wider mb-1.5">Organization Name</label>
                  <input
                    type="text"
                    required
                    value={editOrgName}
                    onChange={(e) => setEditOrgName(e.target.value)}
                    className="w-full rounded-lg border border-slate-700 bg-slate-950 px-3.5 py-2 text-xs text-white focus:outline-none focus:border-blue-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-semibold text-slate-300 uppercase tracking-wider mb-1.5">Description</label>
                  <textarea
                    rows={3}
                    value={editOrgDesc}
                    onChange={(e) => setEditOrgDesc(e.target.value)}
                    className="w-full rounded-lg border border-slate-700 bg-slate-950 px-3.5 py-2 text-xs text-white focus:outline-none focus:border-blue-500 resize-none"
                  />
                </div>
                <button
                  type="submit"
                  disabled={savingSettings}
                  className="rounded-lg bg-blue-600 px-4 py-2 text-xs font-medium text-white hover:bg-blue-500 transition-colors cursor-pointer"
                >
                  {savingSettings ? "Saving..." : "Save Changes"}
                </button>
              </form>
            </div>

            {isOwner && (
              <div className="rounded-xl border border-red-500/30 bg-red-500/5 p-6 shadow-sm">
                <h3 className="text-sm font-bold text-red-400 mb-2">Danger Zone</h3>
                <p className="text-xs text-slate-400 mb-4">
                  Permanently delete this organization, including all member memberships, activity logs, and organization-owned projects.
                </p>
                <button
                  type="button"
                  onClick={handleDeleteOrg}
                  className="rounded-lg bg-red-600 px-4 py-2 text-xs font-medium text-white hover:bg-red-500 transition-colors cursor-pointer"
                >
                  Delete Organization
                </button>
              </div>
            )}
          </div>
        )}
      </main>

      {/* INVITE MEMBER MODAL */}
      {showInviteModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
          <div className="w-full max-w-md rounded-2xl bg-slate-900 border border-slate-800 p-6 shadow-2xl">
            <h3 className="text-base font-bold text-white mb-1">Invite Team Member</h3>
            <p className="text-xs text-slate-400 mb-4">
              Enter the email address of a registered Veracity Maps user.
            </p>
            <form onSubmit={handleInvite} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-slate-300 uppercase tracking-wider mb-1.5">Email Address</label>
                <input
                  type="email"
                  required
                  placeholder="colleague@domain.com"
                  value={inviteEmail}
                  onChange={(e) => setInviteEmail(e.target.value)}
                  className="w-full rounded-lg border border-slate-700 bg-slate-950 px-3.5 py-2 text-xs text-white focus:outline-none focus:border-blue-500"
                />
              </div>
              <div>
                <label className="block text-xs font-semibold text-slate-300 uppercase tracking-wider mb-1.5">Assigned Role</label>
                <select
                  value={inviteRole}
                  onChange={(e) => setInviteRole(e.target.value)}
                  className="w-full rounded-lg border border-slate-700 bg-slate-950 px-3.5 py-2 text-xs text-white focus:outline-none focus:border-blue-500"
                >
                  <option value="admin">Admin (Manage members & projects)</option>
                  <option value="editor">Editor (Create & edit projects)</option>
                  <option value="viewer">Viewer (Read-only inspection)</option>
                </select>
              </div>
              <div className="flex items-center justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setShowInviteModal(false)}
                  className="rounded-lg border border-slate-700 px-4 py-2 text-xs font-medium text-slate-300 hover:bg-slate-800 cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={inviting}
                  className="rounded-lg bg-blue-600 px-4 py-2 text-xs font-medium text-white hover:bg-blue-500 transition-colors cursor-pointer"
                >
                  {inviting ? "Sending..." : "Send Invitation"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* CREATE PROJECT MODAL */}
      {showCreateProjModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
          <div className="w-full max-w-md rounded-2xl bg-slate-900 border border-slate-800 p-6 shadow-2xl">
            <h3 className="text-base font-bold text-white mb-1">Create Organization Project</h3>
            <p className="text-xs text-slate-400 mb-4">
              This project will belong to <strong>{org.name}</strong> and be accessible to all organization members according to their roles.
            </p>
            <form onSubmit={handleCreateProject} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-slate-300 uppercase tracking-wider mb-1.5">Project Name</label>
                <input
                  type="text"
                  required
                  placeholder="e.g. City Airport Masterplan"
                  value={newProjName}
                  onChange={(e) => setNewProjName(e.target.value)}
                  className="w-full rounded-lg border border-slate-700 bg-slate-950 px-3.5 py-2 text-xs text-white focus:outline-none focus:border-blue-500"
                />
              </div>
              <div>
                <label className="block text-xs font-semibold text-slate-300 uppercase tracking-wider mb-1.5">Description (Optional)</label>
                <textarea
                  rows={3}
                  placeholder="Brief overview of map layers and scope..."
                  value={newProjDesc}
                  onChange={(e) => setNewProjDesc(e.target.value)}
                  className="w-full rounded-lg border border-slate-700 bg-slate-950 px-3.5 py-2 text-xs text-white focus:outline-none focus:border-blue-500 resize-none"
                />
              </div>
              <div className="flex items-center justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setShowCreateProjModal(false)}
                  className="rounded-lg border border-slate-700 px-4 py-2 text-xs font-medium text-slate-300 hover:bg-slate-800 cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={creatingProj}
                  className="rounded-lg bg-blue-600 px-4 py-2 text-xs font-medium text-white hover:bg-blue-500 transition-colors cursor-pointer"
                >
                  {creatingProj ? "Creating..." : "Create Project"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* TRANSFER PROJECT MODAL */}
      {showTransferModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
          <div className="w-full max-w-md rounded-2xl bg-slate-900 border border-slate-800 p-6 shadow-2xl">
            <h3 className="text-base font-bold text-white mb-1">Transfer Personal Project to Org</h3>
            <p className="text-xs text-slate-400 mb-4">
              Select one of your personal projects to transfer ownership into <strong>{org.name}</strong>. All associated rasters, floors, and points will be preserved.
            </p>
            {myPersonalProjects.length === 0 ? (
              <div className="py-4 text-xs text-slate-400">
                You have no personal projects available to transfer.
              </div>
            ) : (
              <form onSubmit={handleTransferProject} className="space-y-4">
                <div>
                  <label className="block text-xs font-semibold text-slate-300 uppercase tracking-wider mb-1.5">Select Project</label>
                  <select
                    value={selectedPersonalProjId}
                    onChange={(e) => setSelectedPersonalProjId(e.target.value)}
                    className="w-full rounded-lg border border-slate-700 bg-slate-950 px-3.5 py-2 text-xs text-white focus:outline-none focus:border-blue-500"
                  >
                    {myPersonalProjects.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="flex items-center justify-end gap-2 pt-2">
                  <button
                    type="button"
                    onClick={() => setShowTransferModal(false)}
                    className="rounded-lg border border-slate-700 px-4 py-2 text-xs font-medium text-slate-300 hover:bg-slate-800 cursor-pointer"
                  >
                    Cancel
                  </button>
                  <button
                    type="submit"
                    disabled={transferring}
                    className="rounded-lg bg-blue-600 px-4 py-2 text-xs font-medium text-white hover:bg-blue-500 transition-colors cursor-pointer"
                  >
                    {transferring ? "Transferring..." : "Confirm Transfer"}
                  </button>
                </div>
              </form>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
