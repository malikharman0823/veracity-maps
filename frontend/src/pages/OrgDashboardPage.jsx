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
import {
  Users,
  FolderKanban,
  Settings,
  History,
  Shield,
  UserPlus,
  Trash2,
  Edit,
  ArrowRightLeft,
  Plus,
  Calendar,
  Database,
  Compass,
  ArrowLeft,
  CheckCircle2,
  AlertCircle,
} from "lucide-react"

export default function OrgDashboardPage() {
  const { id: orgId } = useParams()
  const { user } = useAuth()
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
      <div className="flex min-h-screen items-center justify-center bg-[#f5f5f7] text-[#7a7a7a]">
        Loading organization workspace...
      </div>
    )
  }

  if (error || !org) {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center bg-[#f5f5f7] p-6 text-center">
        <AlertCircle className="size-12 text-[#dc2626] mb-4" />
        <h2 className="text-xl font-bold text-[#1d1d1f] mb-2">Access Restricted</h2>
        <p className="text-sm text-[#7a7a7a] mb-6 max-w-md">{error || "Organization not found."}</p>
        <Link
          to="/dashboard"
          className="inline-flex items-center gap-2 rounded-xl bg-[#0066cc] px-4 py-2 text-sm font-medium text-white hover:bg-[#0071e3]"
        >
          <ArrowLeft className="size-4" /> Back to Dashboard
        </Link>
      </div>
    )
  }

  const quotaPercent = Math.min(
    100,
    Math.round(((org.storage_used_bytes || 0) / (org.storage_quota_bytes || 10737418240)) * 100)
  )

  return (
    <div className="min-h-screen bg-[#f5f5f7] text-[#1d1d1f]">
      {/* Header */}
      <header className="border-b border-[#e0e0e0] bg-white sticky top-0 z-20">
        <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-6">
          <div className="flex items-center gap-4">
            <Link
              to="/dashboard"
              className="p-2 rounded-lg hover:bg-[#f5f5f7] text-[#7a7a7a] hover:text-[#1d1d1f] transition-colors"
              title="Back to Dashboard"
            >
              <ArrowLeft className="size-5" />
            </Link>
            <div className="h-9 w-9 rounded-xl bg-[#0066cc]/10 text-[#0066cc] flex items-center justify-center font-bold text-lg shadow-xs">
              {org.name?.charAt(0) || "O"}
            </div>
            <div>
              <div className="flex items-center gap-2.5">
                <h1 className="text-lg font-bold text-[#1d1d1f] tracking-tight">{org.name}</h1>
                <span className="font-mono text-[10px] uppercase font-bold px-2 py-0.5 rounded-full bg-[#0066cc]/10 text-[#0066cc] border border-[#0066cc]/20">
                  {userRole}
                </span>
              </div>
              <p className="text-xs text-[#7a7a7a] line-clamp-1">{org.description || "Organization Workspace"}</p>
            </div>
          </div>

          <div className="flex items-center gap-3">
            {isEditor && (
              <button
                type="button"
                onClick={() => setShowCreateProjModal(true)}
                className="flex items-center gap-2 rounded-xl bg-[#0066cc] px-4 py-2 text-xs font-medium text-white shadow-xs hover:bg-[#0071e3] transition-all cursor-pointer"
              >
                <Plus className="size-4" /> New Org Project
              </button>
            )}
            {isAdmin && (
              <button
                type="button"
                onClick={handleOpenTransferModal}
                className="flex items-center gap-2 rounded-xl border border-[#e0e0e0] bg-white px-3.5 py-2 text-xs font-medium text-[#1d1d1f] shadow-xs hover:bg-[#f5f5f7] transition-all cursor-pointer"
              >
                <ArrowRightLeft className="size-3.5 text-[#7a7a7a]" /> Transfer Project
              </button>
            )}
          </div>
        </div>

        {/* Tab Navigation */}
        <div className="mx-auto flex max-w-7xl gap-8 px-6 text-xs font-medium">
          <button
            onClick={() => setActiveTab("overview")}
            className={`border-b-2 py-3 flex items-center gap-1.5 transition-colors cursor-pointer ${
              activeTab === "overview"
                ? "border-[#0066cc] text-[#0066cc] font-semibold"
                : "border-transparent text-[#7a7a7a] hover:text-[#1d1d1f]"
            }`}
          >
            <Compass className="size-3.5" /> Overview
          </button>
          <button
            onClick={() => setActiveTab("projects")}
            className={`border-b-2 py-3 flex items-center gap-1.5 transition-colors cursor-pointer ${
              activeTab === "projects"
                ? "border-[#0066cc] text-[#0066cc] font-semibold"
                : "border-transparent text-[#7a7a7a] hover:text-[#1d1d1f]"
            }`}
          >
            <FolderKanban className="size-3.5" /> Projects ({projects.length})
          </button>
          <button
            onClick={() => setActiveTab("members")}
            className={`border-b-2 py-3 flex items-center gap-1.5 transition-colors cursor-pointer ${
              activeTab === "members"
                ? "border-[#0066cc] text-[#0066cc] font-semibold"
                : "border-transparent text-[#7a7a7a] hover:text-[#1d1d1f]"
            }`}
          >
            <Users className="size-3.5" /> Members ({members.length})
          </button>
          {isAdmin && (
            <button
              onClick={() => setActiveTab("activity")}
              className={`border-b-2 py-3 flex items-center gap-1.5 transition-colors cursor-pointer ${
                activeTab === "activity"
                  ? "border-[#0066cc] text-[#0066cc] font-semibold"
                  : "border-transparent text-[#7a7a7a] hover:text-[#1d1d1f]"
              }`}
            >
              <History className="size-3.5" /> Activity
            </button>
          )}
          {isAdmin && (
            <button
              onClick={() => setActiveTab("settings")}
              className={`border-b-2 py-3 flex items-center gap-1.5 transition-colors cursor-pointer ${
                activeTab === "settings"
                  ? "border-[#0066cc] text-[#0066cc] font-semibold"
                  : "border-transparent text-[#7a7a7a] hover:text-[#1d1d1f]"
              }`}
            >
              <Settings className="size-3.5" /> Settings
            </button>
          )}
        </div>
      </header>

      {/* Main Container */}
      <main className="mx-auto max-w-7xl p-6">
        {/* TAB 1: OVERVIEW */}
        {activeTab === "overview" && (
          <div className="space-y-6">
            {/* Metric Cards */}
            <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
              <div className="rounded-2xl border border-[#e0e0e0] bg-white p-5 shadow-xs">
                <div className="flex items-center justify-between text-[#7a7a7a] text-xs font-semibold uppercase">
                  <span>Total Projects</span>
                  <FolderKanban className="size-4 text-[#0066cc]" />
                </div>
                <div className="mt-3 text-3xl font-extrabold text-[#1d1d1f]">{projects.length}</div>
                <p className="mt-1 text-[11px] text-[#7a7a7a]">Collaborative map workspaces</p>
              </div>

              <div className="rounded-2xl border border-[#e0e0e0] bg-white p-5 shadow-xs">
                <div className="flex items-center justify-between text-[#7a7a7a] text-xs font-semibold uppercase">
                  <span>Total Members</span>
                  <Users className="size-4 text-[#34c759]" />
                </div>
                <div className="mt-3 text-3xl font-extrabold text-[#1d1d1f]">{members.length}</div>
                <p className="mt-1 text-[11px] text-[#7a7a7a]">Active teammates & collaborators</p>
              </div>

              <div className="rounded-2xl border border-[#e0e0e0] bg-white p-5 shadow-xs">
                <div className="flex items-center justify-between text-[#7a7a7a] text-xs font-semibold uppercase">
                  <span>Storage Used</span>
                  <Database className="size-4 text-[#ff9500]" />
                </div>
                <div className="mt-3 text-2xl font-extrabold text-[#1d1d1f]">
                  {formatBytes(org.storage_used_bytes)}
                </div>
                <div className="mt-2 w-full bg-[#f0f0f2] rounded-full h-1.5 overflow-hidden">
                  <div
                    className="bg-[#0066cc] h-1.5 rounded-full"
                    style={{ width: `${quotaPercent}%` }}
                  />
                </div>
                <p className="mt-1 text-[10px] text-[#7a7a7a] text-right">
                  {quotaPercent}% of {formatBytes(org.storage_quota_bytes || 10737418240)}
                </p>
              </div>

              <div className="rounded-2xl border border-[#e0e0e0] bg-white p-5 shadow-xs">
                <div className="flex items-center justify-between text-[#7a7a7a] text-xs font-semibold uppercase">
                  <span>Your Role</span>
                  <Shield className="size-4 text-[#af52de]" />
                </div>
                <div className="mt-3 text-2xl font-extrabold text-[#1d1d1f] capitalize">{userRole}</div>
                <p className="mt-1 text-[11px] text-[#7a7a7a]">
                  {isOwner ? "Complete ownership" : isAdmin ? "Team administration" : isEditor ? "Full editing access" : "Read-only inspection"}
                </p>
              </div>
            </div>

            {/* Quick Actions & Recent Projects */}
            <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
              <div className="md:col-span-2 rounded-2xl border border-[#e0e0e0] bg-white p-6 shadow-xs">
                <div className="flex items-center justify-between mb-4">
                  <h3 className="text-sm font-bold text-[#1d1d1f]">Organization Projects</h3>
                  <button
                    onClick={() => setActiveTab("projects")}
                    className="text-xs font-semibold text-[#0066cc] hover:underline"
                  >
                    View All →
                  </button>
                </div>
                {projects.length === 0 ? (
                  <div className="py-8 text-center text-xs text-[#7a7a7a]">
                    No projects yet. Create your organization's first mapping project!
                  </div>
                ) : (
                  <div className="space-y-3">
                    {projects.slice(0, 4).map((p) => (
                      <div
                        key={p.id}
                        onClick={() => navigate(`/workspace?project=${encodeURIComponent(p.id)}`)}
                        className="flex items-center justify-between p-3 rounded-xl border border-[#e0e0e0]/70 hover:border-[#0066cc]/50 hover:bg-[#fafafc] transition-all cursor-pointer"
                      >
                        <div className="flex items-center gap-3">
                          <div className="size-8 rounded-lg bg-[#0066cc]/10 text-[#0066cc] flex items-center justify-center">
                            <Compass className="size-4" />
                          </div>
                          <div>
                            <div className="text-xs font-semibold text-[#1d1d1f]">{p.name}</div>
                            <div className="text-[10px] text-[#7a7a7a]">{p.description || "No description"}</div>
                          </div>
                        </div>
                        <div className="text-[10px] text-[#7a7a7a]">
                          Updated {new Date(p.updated_at).toLocaleDateString()}
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              <div className="rounded-2xl border border-[#e0e0e0] bg-white p-6 shadow-xs">
                <h3 className="text-sm font-bold text-[#1d1d1f] mb-4">Workspace Info</h3>
                <div className="space-y-3 text-xs">
                  <div>
                    <span className="text-[#7a7a7a] block text-[11px]">Organization ID</span>
                    <span className="font-mono text-[#1d1d1f] text-[11px] select-all">{org.id}</span>
                  </div>
                  <div>
                    <span className="text-[#7a7a7a] block text-[11px]">Created Date</span>
                    <span className="text-[#1d1d1f]">{new Date(org.created_at).toLocaleDateString()}</span>
                  </div>
                  <div>
                    <span className="text-[#7a7a7a] block text-[11px]">Isolation Level</span>
                    <span className="text-[#34c759] font-medium">Independent Workspace (Zero cross-leak)</span>
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
                <h2 className="text-base font-bold text-[#1d1d1f]">Projects</h2>
                <p className="text-xs text-[#7a7a7a]">Shared geospatial layers, floors, and vector markers.</p>
              </div>
              {isEditor && (
                <button
                  type="button"
                  onClick={() => setShowCreateProjModal(true)}
                  className="flex items-center gap-2 rounded-xl bg-[#0066cc] px-4 py-2 text-xs font-medium text-white shadow-xs hover:bg-[#0071e3] transition-all cursor-pointer"
                >
                  <Plus className="size-4" /> New Org Project
                </button>
              )}
            </div>

            {projects.length === 0 ? (
              <div className="rounded-2xl border border-dashed border-[#e0e0e0] bg-white p-12 text-center">
                <FolderKanban className="size-10 text-[#7a7a7a] mx-auto mb-3" />
                <h3 className="text-sm font-bold text-[#1d1d1f] mb-1">No Projects Found</h3>
                <p className="text-xs text-[#7a7a7a] mb-4">
                  This organization doesn't have any projects yet.
                </p>
                {isEditor && (
                  <button
                    onClick={() => setShowCreateProjModal(true)}
                    className="inline-flex items-center gap-2 rounded-xl bg-[#0066cc] px-4 py-2 text-xs font-medium text-white hover:bg-[#0071e3]"
                  >
                    <Plus className="size-3.5" /> Create Project
                  </button>
                )}
              </div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                {projects.map((p) => (
                  <div
                    key={p.id}
                    onClick={() => navigate(`/workspace?project=${encodeURIComponent(p.id)}`)}
                    className="rounded-2xl border border-[#e0e0e0] bg-white p-5 shadow-xs hover:shadow-md transition-all cursor-pointer flex flex-col justify-between"
                  >
                    <div>
                      <div className="flex items-center justify-between mb-3">
                        <span className="font-mono text-[9px] uppercase px-2 py-0.5 rounded-full bg-[#f5f5f7] text-[#7a7a7a] border border-[#e0e0e0]">
                          Org Project
                        </span>
                        <span className="text-[10px] text-[#7a7a7a]">
                          {new Date(p.updated_at).toLocaleDateString()}
                        </span>
                      </div>
                      <h3 className="text-sm font-bold text-[#1d1d1f] mb-1">{p.name}</h3>
                      <p className="text-xs text-[#7a7a7a] line-clamp-2">{p.description || "No description provided."}</p>
                    </div>

                    <div className="mt-4 pt-3 border-t border-[#e0e0e0]/70 flex items-center justify-between">
                      <span className="text-[11px] font-semibold text-[#0066cc]">Open Project →</span>
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
                <h2 className="text-base font-bold text-[#1d1d1f]">Team Members</h2>
                <p className="text-xs text-[#7a7a7a]">Manage organizational roles and pending member invitations.</p>
              </div>
              {isAdmin && (
                <button
                  type="button"
                  onClick={() => setShowInviteModal(true)}
                  className="flex items-center gap-2 rounded-xl bg-[#0066cc] px-4 py-2 text-xs font-medium text-white shadow-xs hover:bg-[#0071e3] transition-all cursor-pointer"
                >
                  <UserPlus className="size-4" /> Invite Member
                </button>
              )}
            </div>

            <div className="rounded-2xl border border-[#e0e0e0] bg-white overflow-hidden shadow-xs">
              <table className="w-full text-left text-xs border-collapse">
                <thead>
                  <tr className="border-b border-[#e0e0e0] bg-[#fafafc] text-[#7a7a7a] font-semibold">
                    <th className="py-3 px-4">Member</th>
                    <th className="py-3 px-4">Email</th>
                    <th className="py-3 px-4">Status</th>
                    <th className="py-3 px-4">Role</th>
                    {isAdmin && <th className="py-3 px-4 text-right">Actions</th>}
                  </tr>
                </thead>
                <tbody className="divide-y divide-[#e0e0e0]/70">
                  {members.map((m) => {
                    const isSelf = m.user_id === user?.id
                    const isMemberOwner = m.role === "owner"

                    return (
                      <tr key={m.id} className="hover:bg-[#fafafc]">
                        <td className="py-3 px-4 font-medium text-[#1d1d1f]">
                          <div className="flex items-center gap-2.5">
                            <div className="size-7 rounded-full bg-[#0066cc]/10 text-[#0066cc] font-bold flex items-center justify-center text-[11px]">
                              {m.user_name?.charAt(0) || "U"}
                            </div>
                            <span>{m.user_name || "Invited User"} {isSelf && "(You)"}</span>
                          </div>
                        </td>
                        <td className="py-3 px-4 text-[#7a7a7a]">{m.user_email}</td>
                        <td className="py-3 px-4">
                          <span
                            className={`inline-flex items-center gap-1 font-mono text-[10px] uppercase font-bold px-2 py-0.5 rounded-full ${
                              m.invitation_status === "accepted"
                                ? "bg-[#34c759]/10 text-[#34c759]"
                                : "bg-[#ff9500]/10 text-[#ff9500]"
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
                              className="rounded-lg border border-[#e0e0e0] bg-white px-2 py-1 text-xs text-[#1d1d1f] focus:outline-none focus:border-[#0066cc]"
                            >
                              <option value="admin">Admin</option>
                              <option value="editor">Editor</option>
                              <option value="viewer">Viewer</option>
                            </select>
                          ) : (
                            <span className="font-semibold capitalize text-[#1d1d1f]">{m.role}</span>
                          )}
                        </td>
                        {isAdmin && (
                          <td className="py-3 px-4 text-right">
                            {!isMemberOwner && !isSelf && (
                              <button
                                type="button"
                                onClick={() => handleRemoveMember(m)}
                                className="p-1 rounded text-[#7a7a7a] hover:text-[#dc2626] transition-colors cursor-pointer"
                                title="Remove member"
                              >
                                <Trash2 className="size-4" />
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
            <h2 className="text-base font-bold text-[#1d1d1f]">Audit & Activity Logs</h2>
            <div className="rounded-2xl border border-[#e0e0e0] bg-white p-5 shadow-xs">
              {activities.length === 0 ? (
                <div className="py-8 text-center text-xs text-[#7a7a7a]">No activities logged yet.</div>
              ) : (
                <div className="space-y-3">
                  {activities.map((a) => (
                    <div key={a.id} className="flex items-start gap-3 text-xs border-b border-[#e0e0e0]/50 pb-3 last:border-b-0">
                      <div className="size-2 rounded-full bg-[#0066cc] mt-1.5 shrink-0" />
                      <div className="flex-1">
                        <div className="font-semibold text-[#1d1d1f]">{a.user_name}</div>
                        <div className="text-[#7a7a7a] text-[11px]">{a.details}</div>
                      </div>
                      <div className="text-[10px] text-[#7a7a7a] font-mono shrink-0">
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
            <div className="rounded-2xl border border-[#e0e0e0] bg-white p-6 shadow-xs">
              <h2 className="text-base font-bold text-[#1d1d1f] mb-4">Organization Settings</h2>
              <form onSubmit={handleSaveSettings} className="space-y-4">
                <div>
                  <label className="block text-xs font-semibold text-[#1d1d1f] mb-1">Organization Name</label>
                  <input
                    type="text"
                    required
                    value={editOrgName}
                    onChange={(e) => setEditOrgName(e.target.value)}
                    className="w-full rounded-xl border border-[#e0e0e0] px-3.5 py-2 text-xs focus:outline-none focus:border-[#0066cc]"
                  />
                </div>
                <div>
                  <label className="block text-xs font-semibold text-[#1d1d1f] mb-1">Description</label>
                  <textarea
                    rows={3}
                    value={editOrgDesc}
                    onChange={(e) => setEditOrgDesc(e.target.value)}
                    className="w-full rounded-xl border border-[#e0e0e0] px-3.5 py-2 text-xs focus:outline-none focus:border-[#0066cc]"
                  />
                </div>
                <button
                  type="submit"
                  disabled={savingSettings}
                  className="rounded-xl bg-[#0066cc] px-4 py-2 text-xs font-medium text-white hover:bg-[#0071e3] transition-colors cursor-pointer"
                >
                  {savingSettings ? "Saving..." : "Save Changes"}
                </button>
              </form>
            </div>

            {isOwner && (
              <div className="rounded-2xl border border-[#dc2626]/30 bg-red-50/20 p-6 shadow-xs">
                <h3 className="text-sm font-bold text-[#dc2626] mb-2">Danger Zone</h3>
                <p className="text-xs text-[#7a7a7a] mb-4">
                  Permanently delete this organization, including all member memberships, activity logs, and organization-owned projects.
                </p>
                <button
                  type="button"
                  onClick={handleDeleteOrg}
                  className="rounded-xl bg-[#dc2626] px-4 py-2 text-xs font-medium text-white hover:bg-red-700 transition-colors cursor-pointer"
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
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-xs p-4">
          <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-xl border border-[#e0e0e0]">
            <h3 className="text-base font-bold text-[#1d1d1f] mb-1">Invite Team Member</h3>
            <p className="text-xs text-[#7a7a7a] mb-4">
              Enter the email address of a registered Veracity Maps user.
            </p>
            <form onSubmit={handleInvite} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-[#1d1d1f] mb-1">Email Address</label>
                <input
                  type="email"
                  required
                  placeholder="colleague@domain.com"
                  value={inviteEmail}
                  onChange={(e) => setInviteEmail(e.target.value)}
                  className="w-full rounded-xl border border-[#e0e0e0] px-3.5 py-2 text-xs focus:outline-none focus:border-[#0066cc]"
                />
              </div>
              <div>
                <label className="block text-xs font-semibold text-[#1d1d1f] mb-1">Assigned Role</label>
                <select
                  value={inviteRole}
                  onChange={(e) => setInviteRole(e.target.value)}
                  className="w-full rounded-xl border border-[#e0e0e0] bg-white px-3.5 py-2 text-xs focus:outline-none focus:border-[#0066cc]"
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
                  className="rounded-xl border border-[#e0e0e0] px-4 py-2 text-xs font-medium text-[#7a7a7a] hover:bg-[#f5f5f7] cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={inviting}
                  className="rounded-xl bg-[#0066cc] px-4 py-2 text-xs font-medium text-white hover:bg-[#0071e3] transition-colors cursor-pointer"
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
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-xs p-4">
          <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-xl border border-[#e0e0e0]">
            <h3 className="text-base font-bold text-[#1d1d1f] mb-1">Create Organization Project</h3>
            <p className="text-xs text-[#7a7a7a] mb-4">
              This project will belong to <strong>{org.name}</strong> and be accessible to all organization members according to their roles.
            </p>
            <form onSubmit={handleCreateProject} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-[#1d1d1f] mb-1">Project Name</label>
                <input
                  type="text"
                  required
                  placeholder="e.g. City Airport Masterplan"
                  value={newProjName}
                  onChange={(e) => setNewProjName(e.target.value)}
                  className="w-full rounded-xl border border-[#e0e0e0] px-3.5 py-2 text-xs focus:outline-none focus:border-[#0066cc]"
                />
              </div>
              <div>
                <label className="block text-xs font-semibold text-[#1d1d1f] mb-1">Description (Optional)</label>
                <textarea
                  rows={3}
                  placeholder="Brief overview of map layers and scope..."
                  value={newProjDesc}
                  onChange={(e) => setNewProjDesc(e.target.value)}
                  className="w-full rounded-xl border border-[#e0e0e0] px-3.5 py-2 text-xs focus:outline-none focus:border-[#0066cc]"
                />
              </div>
              <div className="flex items-center justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setShowCreateProjModal(false)}
                  className="rounded-xl border border-[#e0e0e0] px-4 py-2 text-xs font-medium text-[#7a7a7a] hover:bg-[#f5f5f7] cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={creatingProj}
                  className="rounded-xl bg-[#0066cc] px-4 py-2 text-xs font-medium text-white hover:bg-[#0071e3] transition-colors cursor-pointer"
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
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-xs p-4">
          <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-xl border border-[#e0e0e0]">
            <h3 className="text-base font-bold text-[#1d1d1f] mb-1">Transfer Personal Project to Org</h3>
            <p className="text-xs text-[#7a7a7a] mb-4">
              Select one of your personal projects to transfer ownership into <strong>{org.name}</strong>. All associated rasters, floors, and points will be preserved.
            </p>
            {myPersonalProjects.length === 0 ? (
              <div className="py-4 text-xs text-[#7a7a7a]">
                You have no personal projects available to transfer.
              </div>
            ) : (
              <form onSubmit={handleTransferProject} className="space-y-4">
                <div>
                  <label className="block text-xs font-semibold text-[#1d1d1f] mb-1">Select Project</label>
                  <select
                    value={selectedPersonalProjId}
                    onChange={(e) => setSelectedPersonalProjId(e.target.value)}
                    className="w-full rounded-xl border border-[#e0e0e0] bg-white px-3.5 py-2 text-xs focus:outline-none focus:border-[#0066cc]"
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
                    className="rounded-xl border border-[#e0e0e0] px-4 py-2 text-xs font-medium text-[#7a7a7a] hover:bg-[#f5f5f7] cursor-pointer"
                  >
                    Cancel
                  </button>
                  <button
                    type="submit"
                    disabled={transferring}
                    className="rounded-xl bg-[#0066cc] px-4 py-2 text-xs font-medium text-white hover:bg-[#0071e3] transition-colors cursor-pointer"
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
