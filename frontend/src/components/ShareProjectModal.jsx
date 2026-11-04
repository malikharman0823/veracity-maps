import React, { useState, useEffect } from "react"
import {
  listProjectMembers,
  shareProject,
  updateProjectMemberRole,
  removeProjectMember,
  listProjectActivities,
} from "../api"

export default function ShareProjectModal({ project, isOpen, onClose }) {
  if (!isOpen || !project) return null

  const [activeTab, setActiveTab] = useState("members") // "members" | "activities"
  const [members, setMembers] = useState([])
  const [activities, setActivities] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")

  // Invite form
  const [email, setEmail] = useState("")
  const [role, setRole] = useState("viewer")
  const [inviting, setInviting] = useState(false)
  const [inviteSuccess, setInviteSuccess] = useState("")

  const loadData = async () => {
    try {
      setLoading(true)
      setError("")
      const [mList, aList] = await Promise.all([
        listProjectMembers(project.id),
        listProjectActivities(project.id).catch(() => []),
      ])
      setMembers(mList)
      setActivities(aList)
    } catch (err) {
      setError(err.response?.data?.error?.message || "Failed to load project members")
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadData()
  }, [project.id])

  const handleInvite = async (e) => {
    e.preventDefault()
    if (!email.trim()) return

    setInviting(true)
    setError("")
    setInviteSuccess("")
    try {
      await shareProject(project.id, email.trim(), role)
      setEmail("")
      setInviteSuccess("Invitation sent successfully!")
      await loadData()
      setTimeout(() => setInviteSuccess(""), 4000)
    } catch (err) {
      setError(err.response?.data?.error?.message || "Failed to send invitation")
    } finally {
      setInviting(false)
    }
  }

  const handleRoleChange = async (memberId, newRole) => {
    try {
      await updateProjectMemberRole(project.id, memberId, newRole)
      setMembers((prev) =>
        prev.map((m) => (m.id === memberId ? { ...m, role: newRole } : m))
      )
    } catch (err) {
      alert("Failed to update role: " + (err.response?.data?.error?.message || err.message))
    }
  }

  const handleRemove = async (member) => {
    const isPending = member.invitation_status === "pending"
    const actionDesc = isPending ? "cancel invitation for" : "remove"
    if (!window.confirm(`Are you sure you want to ${actionDesc} ${member.user_name || member.user_email}?`)) {
      return
    }

    try {
      await removeProjectMember(project.id, member.id)
      setMembers((prev) => prev.filter((m) => m.id !== member.id))
    } catch (err) {
      alert("Failed to remove member: " + (err.response?.data?.error?.message || err.message))
    }
  }

  return (
    <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
      <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-xl w-full p-6 shadow-2xl flex flex-col max-h-[85vh]">
        {/* Header */}
        <div className="flex items-start justify-between gap-4 mb-4 pb-3 border-b border-slate-800">
          <div>
            <div className="flex items-center gap-2">
              <span className="text-xs font-semibold px-2 py-0.5 rounded bg-blue-500/10 text-blue-400 border border-blue-500/20 uppercase tracking-wider">
                Collaborate
              </span>
              <h2 className="text-lg font-bold text-white tracking-tight">Share "{project.name}"</h2>
            </div>
            <p className="text-xs text-slate-400 mt-1">
              Invite registered users to view or edit this project and its GIS layers.
            </p>
          </div>
          <button
            onClick={onClose}
            className="p-1 rounded-lg text-slate-400 hover:text-white hover:bg-slate-800 transition-colors"
          >
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Tab Selector */}
        <div className="flex items-center gap-2 mb-4">
          <button
            onClick={() => setActiveTab("members")}
            className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
              activeTab === "members"
                ? "bg-blue-600 text-white"
                : "bg-slate-800/60 text-slate-400 hover:text-white"
            }`}
          >
            Collaborators ({members.length})
          </button>
          <button
            onClick={() => setActiveTab("activities")}
            className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
              activeTab === "activities"
                ? "bg-blue-600 text-white"
                : "bg-slate-800/60 text-slate-400 hover:text-white"
            }`}
          >
            Activity History ({activities.length})
          </button>
        </div>

        {error && (
          <div className="mb-4 p-3 rounded-lg bg-red-500/10 border border-red-500/20 text-red-400 text-xs flex items-center gap-2">
            <span>{error}</span>
          </div>
        )}

        {inviteSuccess && (
          <div className="mb-4 p-3 rounded-lg bg-emerald-500/10 border border-emerald-500/20 text-emerald-400 text-xs flex items-center gap-2">
            <span>{inviteSuccess}</span>
          </div>
        )}

        {activeTab === "members" ? (
          <>
            {/* Invite Form */}
            <form onSubmit={handleInvite} className="mb-5 p-3 rounded-xl bg-slate-950/60 border border-slate-800">
              <label className="block text-[11px] font-semibold text-slate-300 uppercase tracking-wider mb-2">
                Invite by Registered Email
              </label>
              <div className="flex items-center gap-2">
                <input
                  type="email"
                  required
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="collaborator@organization.com"
                  className="flex-1 px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-700 text-white text-xs focus:outline-none focus:border-blue-500"
                />
                <select
                  value={role}
                  onChange={(e) => setRole(e.target.value)}
                  className="px-2.5 py-1.5 rounded-lg bg-slate-900 border border-slate-700 text-white text-xs focus:outline-none focus:border-blue-500"
                >
                  <option value="viewer">Viewer (Read-only)</option>
                  <option value="editor">Editor (Can edit & upload)</option>
                </select>
                <button
                  type="submit"
                  disabled={inviting || !email.trim()}
                  className="px-3.5 py-1.5 rounded-lg bg-blue-600 hover:bg-blue-500 text-white text-xs font-medium transition-colors disabled:opacity-50 shrink-0"
                >
                  {inviting ? "Inviting..." : "Send Invite"}
                </button>
              </div>
            </form>

            {/* Member List */}
            <div className="flex-1 overflow-y-auto space-y-2 pr-1">
              {loading ? (
                <div className="text-center py-8 text-xs text-slate-500">Loading members...</div>
              ) : members.length === 0 ? (
                <div className="text-center py-8 text-xs text-slate-500">No members found.</div>
              ) : (
                members.map((m) => {
                  const isOwner = m.role === "owner"
                  const isPending = m.invitation_status === "pending"

                  return (
                    <div
                      key={m.id}
                      className="p-3 rounded-lg bg-slate-950/40 border border-slate-800/80 flex items-center justify-between gap-3"
                    >
                      <div className="flex items-center gap-2.5 overflow-hidden">
                        <div className="w-8 h-8 rounded-full bg-slate-800 border border-slate-700 flex items-center justify-center text-xs font-semibold text-slate-300 uppercase shrink-0">
                          {m.user_name?.charAt(0) || m.user_email?.charAt(0) || "U"}
                        </div>
                        <div className="truncate">
                          <div className="flex items-center gap-2">
                            <span className="text-xs font-medium text-white truncate">
                              {m.user_name || "Invited User"}
                            </span>
                            {isOwner && (
                              <span className="text-[10px] px-1.5 py-0.2 rounded bg-amber-500/10 text-amber-400 border border-amber-500/20 font-medium">
                                Owner
                              </span>
                            )}
                            {isPending && (
                              <span className="text-[10px] px-1.5 py-0.2 rounded bg-amber-500/10 text-amber-400 border border-amber-500/20 font-medium animate-pulse">
                                Pending
                              </span>
                            )}
                          </div>
                          <div className="text-[11px] text-slate-400 truncate">{m.user_email}</div>
                        </div>
                      </div>

                      <div className="flex items-center gap-2 shrink-0">
                        {isOwner ? (
                          <span className="text-xs text-slate-500 italic pr-2">Full Access</span>
                        ) : (
                          <>
                            <select
                              value={m.role}
                              onChange={(e) => handleRoleChange(m.id, e.target.value)}
                              className="px-2 py-1 rounded bg-slate-900 border border-slate-700 text-white text-xs focus:outline-none focus:border-blue-500"
                            >
                              <option value="viewer">Viewer</option>
                              <option value="editor">Editor</option>
                            </select>
                            <button
                              onClick={() => handleRemove(m)}
                              title={isPending ? "Cancel Invitation" : "Remove Collaborator"}
                              className="p-1 rounded hover:bg-red-500/20 text-slate-400 hover:text-red-400 transition-colors"
                            >
                              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                              </svg>
                            </button>
                          </>
                        )}
                      </div>
                    </div>
                  )
                })
              )}
            </div>
          </>
        ) : (
          /* Activity Log Tab */
          <div className="flex-1 overflow-y-auto space-y-2 pr-1">
            {activities.length === 0 ? (
              <div className="text-center py-10 text-xs text-slate-500">No project activity recorded yet.</div>
            ) : (
              activities.map((act) => (
                <div
                  key={act.id}
                  className="p-3 rounded-lg bg-slate-950/40 border border-slate-800/80 flex items-start gap-3"
                >
                  <div className="w-2 h-2 rounded-full bg-blue-500 mt-1.5 shrink-0"></div>
                  <div className="flex-1">
                    <div className="text-xs text-slate-200">
                      <span className="font-semibold text-white">{act.user_name}</span>: {act.details}
                    </div>
                    <div className="text-[10px] text-slate-500 mt-1">
                      {new Date(act.timestamp).toLocaleString()}
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>
        )}

        {/* Footer */}
        <div className="mt-4 pt-3 border-t border-slate-800 flex justify-end">
          <button
            onClick={onClose}
            className="px-4 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-white text-xs font-medium transition-colors"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  )
}
