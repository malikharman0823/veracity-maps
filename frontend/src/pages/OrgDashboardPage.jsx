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
import ThemeToggle from "../components/ThemeToggle"

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
      <div className="min-h-screen bg-white dark:bg-[#1c1c1c] text-neutral-500 dark:text-neutral-400 flex items-center justify-center text-sm transition-colors">
        <svg className="animate-spin h-6 w-6 text-[#3ecf8e] mr-2" fill="none" viewBox="0 0 24 24">
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
          <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
        </svg>
        Loading organization workspace...
      </div>
    )
  }

  if (error || !org) {
    return (
      <div className="min-h-screen bg-white dark:bg-[#1c1c1c] text-neutral-900 dark:text-white flex flex-col items-center justify-center p-6 text-center transition-colors">
        <div className="w-12 h-12 rounded-[6px] bg-red-500/10 border border-red-500/20 text-red-500 dark:text-red-400 flex items-center justify-center mb-4">
          <svg className="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
          </svg>
        </div>
        <h2 className="text-xl font-bold mb-2">Access Restricted</h2>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mb-6 max-w-md">{error || "Organization not found."}</p>
        <Link
          to="/dashboard"
          className="inline-flex items-center gap-2 rounded-[6px] bg-[#3ecf8e] px-4 py-2 text-sm font-medium text-[#171717] hover:bg-[#24b47e] transition-colors shadow-xs"
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
    <div className="min-h-screen bg-white dark:bg-[#1c1c1c] text-neutral-900 dark:text-neutral-100 flex flex-col transition-colors">
      {/* Top Header */}
      <header className="h-16 border-b border-neutral-200 dark:border-[#2e2e2e] bg-white/80 dark:bg-[#1c1c1c]/80 backdrop-blur-md px-6 flex items-center justify-between sticky top-0 z-30">
        <div className="flex items-center gap-4">
          <Link
            to="/dashboard"
            className="p-1.5 rounded-[6px] bg-white dark:bg-[#202020] hover:bg-neutral-100 dark:hover:bg-[#282828] text-neutral-700 dark:text-neutral-300 border border-neutral-300 dark:border-[#2e2e2e] transition-colors shadow-xs"
            title="Back to Dashboard"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 19l-7-7m0 0l7-7m-7 7h18" />
            </svg>
          </Link>
          <div className="w-8 h-8 rounded-[6px] bg-[#3ecf8e]/10 border border-[#3ecf8e]/30 flex items-center justify-center text-[#24b47e] dark:text-[#3ecf8e] font-bold">
            {org.name?.charAt(0) || "O"}
          </div>
          <div>
            <div className="flex items-center gap-2">
              <span className="font-semibold text-lg text-neutral-900 dark:text-white tracking-tight">{org.name}</span>
              <span className="text-[10px] px-2 py-0.5 rounded-[4px] bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30 font-semibold uppercase">
                {userRole}
              </span>
            </div>
            <p className="text-xs text-neutral-500 dark:text-neutral-400 line-clamp-1">{org.description || "Organization Workspace"}</p>
          </div>
        </div>

        <div className="flex items-center gap-2.5">
          <ThemeToggle />
          {isEditor && (
            <button
              type="button"
              onClick={() => setShowCreateProjModal(true)}
              className="inline-flex items-center gap-2 px-3.5 py-2 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-medium text-xs transition-colors shadow-xs cursor-pointer"
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
              className="inline-flex items-center gap-2 px-3 py-2 rounded-[6px] bg-white dark:bg-[#202020] hover:bg-neutral-100 dark:hover:bg-[#282828] text-neutral-800 dark:text-neutral-200 border border-neutral-300 dark:border-[#2e2e2e] text-xs font-medium transition-colors cursor-pointer shadow-xs"
            >
              <svg className="w-3.5 h-3.5 text-neutral-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4" />
              </svg>
              Transfer Project
            </button>
          )}
          <button
            onClick={logout}
            className="px-3 py-1.5 rounded-[6px] text-xs font-medium border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] text-neutral-700 dark:text-neutral-300 hover:text-red-500 dark:hover:text-red-400 hover:border-red-500/30 transition-colors cursor-pointer"
          >
            Log out
          </button>
        </div>
      </header>

      {/* Main Content Area */}
      <main className="flex-1 max-w-7xl w-full mx-auto px-6 py-6">
        {/* Section Tabs */}
        <div className="flex items-center gap-1.5 border-b border-neutral-200 dark:border-[#2e2e2e] pb-3 mb-6 overflow-x-auto">
          <button
            onClick={() => setActiveTab("overview")}
            className={`px-3.5 py-1.5 rounded-[6px] text-xs font-medium transition-all cursor-pointer ${
              activeTab === "overview"
                ? "bg-[#3ecf8e] text-[#171717] shadow-xs"
                : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#242424]"
            }`}
          >
            Overview
          </button>
          <button
            onClick={() => setActiveTab("projects")}
            className={`px-3.5 py-1.5 rounded-[6px] text-xs font-medium transition-all cursor-pointer ${
              activeTab === "projects"
                ? "bg-[#3ecf8e] text-[#171717] shadow-xs"
                : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#242424]"
            }`}
          >
            Projects ({projects.length})
          </button>
          <button
            onClick={() => setActiveTab("members")}
            className={`px-3.5 py-1.5 rounded-[6px] text-xs font-medium transition-all cursor-pointer ${
              activeTab === "members"
                ? "bg-[#3ecf8e] text-[#171717] shadow-xs"
                : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#242424]"
            }`}
          >
            Members ({members.length})
          </button>
          {isAdmin && (
            <button
              onClick={() => setActiveTab("activity")}
              className={`px-3.5 py-1.5 rounded-[6px] text-xs font-medium transition-all cursor-pointer ${
                activeTab === "activity"
                  ? "bg-[#3ecf8e] text-[#171717] shadow-xs"
                  : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#242424]"
              }`}
            >
              Activity Logs
            </button>
          )}
          {isAdmin && (
            <button
              onClick={() => setActiveTab("settings")}
              className={`px-3.5 py-1.5 rounded-[6px] text-xs font-medium transition-all cursor-pointer ${
                activeTab === "settings"
                  ? "bg-[#3ecf8e] text-[#171717] shadow-xs"
                  : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#242424]"
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
              <div className="rounded-[12px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] p-5 shadow-xs">
                <div className="flex items-center justify-between text-neutral-500 dark:text-neutral-400 text-xs font-semibold uppercase tracking-wider">
                  <span>Total Projects</span>
                  <span className="text-[#3ecf8e] font-bold">#</span>
                </div>
                <div className="mt-3 text-3xl font-extrabold text-neutral-900 dark:text-white">{projects.length}</div>
                <p className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">Collaborative map workspaces</p>
              </div>

              <div className="rounded-[12px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] p-5 shadow-xs">
                <div className="flex items-center justify-between text-neutral-500 dark:text-neutral-400 text-xs font-semibold uppercase tracking-wider">
                  <span>Total Members</span>
                  <span className="text-[#3ecf8e] font-bold">●</span>
                </div>
                <div className="mt-3 text-3xl font-extrabold text-neutral-900 dark:text-white">{members.length}</div>
                <p className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">Active teammates & collaborators</p>
              </div>

              <div className="rounded-[12px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] p-5 shadow-xs">
                <div className="flex items-center justify-between text-neutral-500 dark:text-neutral-400 text-xs font-semibold uppercase tracking-wider">
                  <span>Storage Used</span>
                  <span className="text-[#ffdb13] font-bold">MB</span>
                </div>
                <div className="mt-3 text-2xl font-extrabold text-neutral-900 dark:text-white">
                  {formatBytes(org.storage_used_bytes)}
                </div>
                <div className="mt-2 w-full bg-neutral-100 dark:bg-[#171717] rounded-full h-1.5 overflow-hidden">
                  <div
                    className="bg-[#3ecf8e] h-1.5 rounded-full"
                    style={{ width: `${quotaPercent}%` }}
                  />
                </div>
                <p className="mt-1 text-[10px] text-neutral-500 dark:text-neutral-400 text-right">
                  {quotaPercent}% of {formatBytes(org.storage_quota_bytes || 10737418240)}
                </p>
              </div>

              <div className="rounded-[12px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] p-5 shadow-xs">
                <div className="flex items-center justify-between text-neutral-500 dark:text-neutral-400 text-xs font-semibold uppercase tracking-wider">
                  <span>Your Role</span>
                  <span className="text-[#3ecf8e] font-bold">★</span>
                </div>
                <div className="mt-3 text-2xl font-extrabold text-neutral-900 dark:text-white capitalize">{userRole}</div>
                <p className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
                  {isOwner ? "Complete ownership" : isAdmin ? "Team administration" : isEditor ? "Full editing access" : "Read-only inspection"}
                </p>
              </div>
            </div>

            {/* Quick Actions & Recent Projects */}
            <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
              <div className="md:col-span-2 rounded-[12px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] p-6 shadow-xs">
                <div className="flex items-center justify-between mb-4">
                  <h3 className="text-base font-semibold text-neutral-900 dark:text-white">Organization Projects</h3>
                  <button
                    onClick={() => setActiveTab("projects")}
                    className="text-xs font-semibold text-[#24b47e] dark:text-[#3ecf8e] hover:underline cursor-pointer"
                  >
                    View All →
                  </button>
                </div>
                {projects.length === 0 ? (
                  <div className="py-8 text-center text-xs text-neutral-500 dark:text-neutral-400">
                    No projects yet. Create your organization's first mapping project!
                  </div>
                ) : (
                  <div className="space-y-3">
                    {projects.slice(0, 4).map((p) => (
                      <div
                        key={p.id}
                        onClick={() => navigate(`/workspace?project=${encodeURIComponent(p.id)}`)}
                        className="flex items-center justify-between p-3.5 rounded-[8px] border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] hover:bg-neutral-100 dark:hover:bg-[#242424] hover:border-[#3ecf8e] transition-all cursor-pointer shadow-2xs"
                      >
                        <div className="flex items-center gap-3">
                          <div className="w-8 h-8 rounded-[6px] bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] flex items-center justify-center font-bold text-xs border border-[#3ecf8e]/30">
                            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l4.553 2.276A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7" />
                            </svg>
                          </div>
                          <div>
                            <div className="text-sm font-semibold text-neutral-900 dark:text-white">{p.name}</div>
                            <div className="text-xs text-neutral-500 dark:text-neutral-400 line-clamp-1">{p.description || "No description"}</div>
                          </div>
                        </div>
                        <div className="text-[11px] text-neutral-500 dark:text-neutral-400">
                          Updated {new Date(p.updated_at).toLocaleDateString()}
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              <div className="rounded-[12px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] p-6 shadow-xs">
                <h3 className="text-base font-semibold text-neutral-900 dark:text-white mb-4">Workspace Info</h3>
                <div className="space-y-3.5 text-xs">
                  <div>
                    <span className="text-neutral-500 dark:text-neutral-400 block text-[11px] uppercase tracking-wider mb-0.5">Organization ID</span>
                    <span className="font-mono text-neutral-800 dark:text-neutral-200 select-all bg-neutral-100 dark:bg-[#171717] px-2 py-1 rounded-[4px] border border-neutral-200 dark:border-[#2e2e2e] block text-[11px] truncate">{org.id}</span>
                  </div>
                  <div>
                    <span className="text-neutral-500 dark:text-neutral-400 block text-[11px] uppercase tracking-wider mb-0.5">Created Date</span>
                    <span className="text-neutral-800 dark:text-neutral-200">{new Date(org.created_at).toLocaleDateString()}</span>
                  </div>
                  <div>
                    <span className="text-neutral-500 dark:text-neutral-400 block text-[11px] uppercase tracking-wider mb-0.5">Isolation Level</span>
                    <span className="text-[#24b47e] dark:text-[#3ecf8e] font-medium">Independent Workspace</span>
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
                <h2 className="text-lg font-bold text-neutral-900 dark:text-white">Projects</h2>
                <p className="text-xs text-neutral-500 dark:text-neutral-400">Shared geospatial layers, floors, and vector markers.</p>
              </div>
              {isEditor && (
                <button
                  type="button"
                  onClick={() => setShowCreateProjModal(true)}
                  className="inline-flex items-center gap-2 px-3.5 py-2 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-medium text-xs transition-colors shadow-xs cursor-pointer"
                >
                  <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                  </svg>
                  New Org Project
                </button>
              )}
            </div>

            {projects.length === 0 ? (
              <div className="rounded-[12px] border border-dashed border-neutral-300 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] p-12 text-center">
                <h3 className="text-base font-semibold text-neutral-900 dark:text-white mb-1">No Projects Found</h3>
                <p className="text-xs text-neutral-500 dark:text-neutral-400 mb-4">
                  This organization doesn't have any projects yet.
                </p>
                {isEditor && (
                  <button
                    onClick={() => setShowCreateProjModal(true)}
                    className="inline-flex items-center gap-2 rounded-[6px] bg-[#3ecf8e] px-4 py-2 text-xs font-semibold text-[#171717] hover:bg-[#24b47e] transition-colors cursor-pointer shadow-xs"
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
                    className="group relative bg-white dark:bg-[#202020] hover:bg-neutral-50 dark:hover:bg-[#242424] border border-neutral-200 dark:border-[#2e2e2e] hover:border-[#3ecf8e] dark:hover:border-[#3ecf8e] rounded-[12px] p-5 transition-all duration-200 cursor-pointer flex flex-col justify-between shadow-xs hover:shadow-md"
                  >
                    <div>
                      <div className="flex items-start justify-between gap-3 mb-2">
                        <div className="flex items-center gap-2">
                          <span className="text-[10px] px-1.5 py-0.5 rounded-[4px] bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30 font-semibold uppercase">
                            Org Project
                          </span>
                          <h3 className="text-base font-semibold text-neutral-900 dark:text-white group-hover:text-[#24b47e] dark:group-hover:text-[#3ecf8e] transition-colors line-clamp-1">
                            {p.name}
                          </h3>
                        </div>
                      </div>
                      <p className="text-xs text-neutral-500 dark:text-neutral-400 line-clamp-2 min-h-[32px]">
                        {p.description || "No description provided."}
                      </p>
                    </div>

                    <div className="mt-6 pt-4 border-t border-neutral-100 dark:border-[#2e2e2e] flex items-center justify-between text-[11px] text-neutral-500 dark:text-neutral-400">
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
                <h2 className="text-lg font-bold text-neutral-900 dark:text-white">Team Members</h2>
                <p className="text-xs text-neutral-500 dark:text-neutral-400">Manage organizational roles and pending member invitations.</p>
              </div>
              {isAdmin && (
                <button
                  type="button"
                  onClick={() => setShowInviteModal(true)}
                  className="inline-flex items-center gap-2 px-3.5 py-2 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-semibold text-xs transition-colors shadow-xs cursor-pointer"
                >
                  <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                  </svg>
                  Invite Member
                </button>
              )}
            </div>

            <div className="rounded-[12px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] overflow-hidden shadow-xs">
              <table className="w-full text-left text-xs border-collapse">
                <thead>
                  <tr className="border-b border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] text-neutral-600 dark:text-neutral-400 font-semibold">
                    <th className="py-3 px-4">Member</th>
                    <th className="py-3 px-4">Email</th>
                    <th className="py-3 px-4">Status</th>
                    <th className="py-3 px-4">Role</th>
                    {isAdmin && <th className="py-3 px-4 text-right">Actions</th>}
                  </tr>
                </thead>
                <tbody className="divide-y divide-neutral-200 dark:divide-[#2e2e2e]">
                  {members.map((m) => {
                    const isSelf = m.user_id === user?.id
                    const isMemberOwner = m.role === "owner"

                    return (
                      <tr key={m.id} className="hover:bg-neutral-50 dark:hover:bg-[#282828] transition-colors">
                        <td className="py-3 px-4 font-medium text-neutral-900 dark:text-white">
                          <div className="flex items-center gap-2.5">
                            <div className="w-7 h-7 rounded-full bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] font-bold flex items-center justify-center text-[11px] border border-[#3ecf8e]/30">
                              {m.user_name?.charAt(0) || "U"}
                            </div>
                            <span>{m.user_name || "Invited User"} {isSelf && "(You)"}</span>
                          </div>
                        </td>
                        <td className="py-3 px-4 text-neutral-500 dark:text-neutral-400">{m.user_email}</td>
                        <td className="py-3 px-4">
                          <span
                            className={`inline-flex items-center gap-1 font-mono text-[10px] uppercase font-bold px-2 py-0.5 rounded-[4px] ${
                              m.invitation_status === "accepted"
                                ? "bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30"
                                : "bg-amber-500/10 text-amber-500 border border-amber-500/30"
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
                              className="rounded-[6px] border border-neutral-300 dark:border-[#2e2e2e] bg-white dark:bg-[#171717] px-2 py-1 text-xs text-neutral-900 dark:text-white focus:outline-none focus:ring-1 focus:ring-[#3ecf8e]"
                            >
                              <option value="admin">Admin</option>
                              <option value="editor">Editor</option>
                              <option value="viewer">Viewer</option>
                            </select>
                          ) : (
                            <span className="font-semibold capitalize text-neutral-900 dark:text-white">{m.role}</span>
                          )}
                        </td>
                        {isAdmin && (
                          <td className="py-3 px-4 text-right">
                            {!isMemberOwner && !isSelf && (
                              <button
                                type="button"
                                onClick={() => handleRemoveMember(m)}
                                className="p-1 rounded-[4px] text-neutral-400 hover:text-red-500 transition-colors cursor-pointer"
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
            <h2 className="text-lg font-bold text-neutral-900 dark:text-white">Audit & Activity Logs</h2>
            <div className="rounded-[12px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] p-5 shadow-xs">
              {activities.length === 0 ? (
                <div className="py-8 text-center text-xs text-neutral-500 dark:text-neutral-400">No activities logged yet.</div>
              ) : (
                <div className="space-y-3">
                  {activities.map((a) => (
                    <div key={a.id} className="flex items-start gap-3 text-xs border-b border-neutral-200 dark:border-[#2e2e2e] pb-3 last:border-b-0">
                      <div className="w-2 h-2 rounded-full bg-[#3ecf8e] mt-1.5 shrink-0" />
                      <div className="flex-1">
                        <div className="font-semibold text-neutral-900 dark:text-white">{a.user_name}</div>
                        <div className="text-neutral-500 dark:text-neutral-400 text-xs mt-0.5">{a.details}</div>
                      </div>
                      <div className="text-[10px] text-neutral-400 font-mono shrink-0">
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
            <div className="rounded-[12px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] p-6 shadow-xs">
              <h2 className="text-base font-bold text-neutral-900 dark:text-white mb-4">Organization Settings</h2>
              <form onSubmit={handleSaveSettings} className="space-y-4">
                <div>
                  <label className="block text-xs font-semibold text-neutral-700 dark:text-neutral-300 uppercase tracking-wider mb-1.5">Organization Name</label>
                  <input
                    type="text"
                    required
                    value={editOrgName}
                    onChange={(e) => setEditOrgName(e.target.value)}
                    className="w-full rounded-[6px] border border-neutral-300 dark:border-[#2e2e2e] bg-white dark:bg-[#171717] px-3.5 py-2 text-xs text-neutral-900 dark:text-white focus:outline-none focus:ring-1 focus:ring-[#3ecf8e]"
                  />
                </div>
                <div>
                  <label className="block text-xs font-semibold text-neutral-700 dark:text-neutral-300 uppercase tracking-wider mb-1.5">Description</label>
                  <textarea
                    rows={3}
                    value={editOrgDesc}
                    onChange={(e) => setEditOrgDesc(e.target.value)}
                    className="w-full rounded-[6px] border border-neutral-300 dark:border-[#2e2e2e] bg-white dark:bg-[#171717] px-3.5 py-2 text-xs text-neutral-900 dark:text-white focus:outline-none focus:ring-1 focus:ring-[#3ecf8e] resize-none"
                  />
                </div>
                <button
                  type="submit"
                  disabled={savingSettings}
                  className="rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] px-4 py-2 text-xs font-semibold text-[#171717] transition-colors cursor-pointer shadow-xs"
                >
                  {savingSettings ? "Saving..." : "Save Changes"}
                </button>
              </form>
            </div>

            {isOwner && (
              <div className="rounded-[12px] border border-red-500/30 bg-red-500/5 p-6 shadow-xs">
                <h3 className="text-sm font-bold text-red-500 dark:text-red-400 mb-2">Danger Zone</h3>
                <p className="text-xs text-neutral-500 dark:text-neutral-400 mb-4">
                  Permanently delete this organization, including all member memberships, activity logs, and organization-owned projects.
                </p>
                <button
                  type="button"
                  onClick={handleDeleteOrg}
                  className="rounded-[6px] bg-red-600 px-4 py-2 text-xs font-medium text-white hover:bg-red-500 transition-colors cursor-pointer"
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
          <div className="w-full max-w-md rounded-[12px] bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] text-neutral-900 dark:text-white p-6 shadow-2xl">
            <h3 className="text-base font-bold text-neutral-900 dark:text-white mb-1">Invite Team Member</h3>
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mb-4">
              Enter the email address of a registered Veracity Maps user.
            </p>
            <form onSubmit={handleInvite} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-neutral-700 dark:text-neutral-300 uppercase tracking-wider mb-1.5">Email Address</label>
                <input
                  type="email"
                  required
                  placeholder="colleague@domain.com"
                  value={inviteEmail}
                  onChange={(e) => setInviteEmail(e.target.value)}
                  className="w-full rounded-[6px] border border-neutral-300 dark:border-[#2e2e2e] bg-white dark:bg-[#171717] px-3.5 py-2 text-xs text-neutral-900 dark:text-white focus:outline-none focus:ring-1 focus:ring-[#3ecf8e]"
                />
              </div>
              <div>
                <label className="block text-xs font-semibold text-neutral-700 dark:text-neutral-300 uppercase tracking-wider mb-1.5">Assigned Role</label>
                <select
                  value={inviteRole}
                  onChange={(e) => setInviteRole(e.target.value)}
                  className="w-full rounded-[6px] border border-neutral-300 dark:border-[#2e2e2e] bg-white dark:bg-[#171717] px-3.5 py-2 text-xs text-neutral-900 dark:text-white focus:outline-none focus:ring-1 focus:ring-[#3ecf8e]"
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
                  className="rounded-[6px] border border-neutral-300 dark:border-[#2e2e2e] px-4 py-2 text-xs font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#282828] cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={inviting}
                  className="rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] px-4 py-2 text-xs font-semibold text-[#171717] transition-colors cursor-pointer shadow-xs"
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
          <div className="w-full max-w-md rounded-[12px] bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] text-neutral-900 dark:text-white p-6 shadow-2xl">
            <h3 className="text-base font-bold text-neutral-900 dark:text-white mb-1">Create Organization Project</h3>
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mb-4">
              This project will belong to <strong>{org.name}</strong> and be accessible to all organization members according to their roles.
            </p>
            <form onSubmit={handleCreateProject} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-neutral-700 dark:text-neutral-300 uppercase tracking-wider mb-1.5">Project Name</label>
                <input
                  type="text"
                  required
                  placeholder="e.g. City Airport Masterplan"
                  value={newProjName}
                  onChange={(e) => setNewProjName(e.target.value)}
                  className="w-full rounded-[6px] border border-neutral-300 dark:border-[#2e2e2e] bg-white dark:bg-[#171717] px-3.5 py-2 text-xs text-neutral-900 dark:text-white focus:outline-none focus:ring-1 focus:ring-[#3ecf8e]"
                />
              </div>
              <div>
                <label className="block text-xs font-semibold text-neutral-700 dark:text-neutral-300 uppercase tracking-wider mb-1.5">Description (Optional)</label>
                <textarea
                  rows={3}
                  placeholder="Brief overview of map layers and scope..."
                  value={newProjDesc}
                  onChange={(e) => setNewProjDesc(e.target.value)}
                  className="w-full rounded-[6px] border border-neutral-300 dark:border-[#2e2e2e] bg-white dark:bg-[#171717] px-3.5 py-2 text-xs text-neutral-900 dark:text-white focus:outline-none focus:ring-1 focus:ring-[#3ecf8e] resize-none"
                />
              </div>
              <div className="flex items-center justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setShowCreateProjModal(false)}
                  className="rounded-[6px] border border-neutral-300 dark:border-[#2e2e2e] px-4 py-2 text-xs font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#282828] cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={creatingProj}
                  className="rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] px-4 py-2 text-xs font-semibold text-[#171717] transition-colors cursor-pointer shadow-xs"
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
          <div className="w-full max-w-md rounded-[12px] bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] text-neutral-900 dark:text-white p-6 shadow-2xl">
            <h3 className="text-base font-bold text-neutral-900 dark:text-white mb-1">Transfer Personal Project to Org</h3>
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mb-4">
              Select one of your personal projects to transfer ownership into <strong>{org.name}</strong>. All associated rasters, floors, and points will be preserved.
            </p>
            {myPersonalProjects.length === 0 ? (
              <div className="py-4 text-xs text-neutral-500 dark:text-neutral-400">
                You have no personal projects available to transfer.
              </div>
            ) : (
              <form onSubmit={handleTransferProject} className="space-y-4">
                <div>
                  <label className="block text-xs font-semibold text-neutral-700 dark:text-neutral-300 uppercase tracking-wider mb-1.5">Select Project</label>
                  <select
                    value={selectedPersonalProjId}
                    onChange={(e) => setSelectedPersonalProjId(e.target.value)}
                    className="w-full rounded-[6px] border border-neutral-300 dark:border-[#2e2e2e] bg-white dark:bg-[#171717] px-3.5 py-2 text-xs text-neutral-900 dark:text-white focus:outline-none focus:ring-1 focus:ring-[#3ecf8e]"
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
                    className="rounded-[6px] border border-neutral-300 dark:border-[#2e2e2e] px-4 py-2 text-xs font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#282828] cursor-pointer"
                  >
                    Cancel
                  </button>
                  <button
                    type="submit"
                    disabled={transferring}
                    className="rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] px-4 py-2 text-xs font-semibold text-[#171717] transition-colors cursor-pointer shadow-xs"
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
