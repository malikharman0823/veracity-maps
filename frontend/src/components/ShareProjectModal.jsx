import React, { useState, useEffect } from "react"
import { Share2, Copy, Check } from "lucide-react"
import {
  listProjectMembers,
  shareProject,
  removeProjectMember,
  listProjectActivities,
} from "../api"

const formatDateTime = (dateVal) => {
  if (!dateVal) return "N/A"
  try {
    let cleanVal = dateVal
    if (typeof cleanVal === "string") {
      cleanVal = cleanVal.replace(/\.(\d{3})\d+([+-].*|Z)?$/, (match, p1, p2) => "." + p1 + (p2 || ""))
      if (/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}/.test(cleanVal)) {
        cleanVal = cleanVal.replace(" ", "T")
      }
    }
    const d = new Date(cleanVal)
    if (isNaN(d.getTime()) || d.getFullYear() <= 1) return "N/A"
    const formatted = d.toLocaleString(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    })
    return formatted === "Invalid Date" ? "N/A" : formatted
  } catch {
    return "N/A"
  }
}

export default function ShareProjectModal({ project, isOpen, onClose }) {
  if (!isOpen || !project) return null

  const [activeTab, setActiveTab] = useState("members") // "members" | "activities"
  const [members, setMembers] = useState([])
  const [activities, setActivities] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")

  // Invite form
  const [email, setEmail] = useState("")
  const [inviting, setInviting] = useState(false)
  const [inviteSuccess, setInviteSuccess] = useState("")
  const [copiedLink, setCopiedLink] = useState(false)

  const shareUrl = `${window.location.origin}/workspace?project=${encodeURIComponent(project.id)}`

  const handleCopyLink = () => {
    navigator.clipboard.writeText(shareUrl)
    setCopiedLink(true)
    setTimeout(() => setCopiedLink(false), 2500)
  }

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
    if (!email.trim() || members.length >= 10) return

    setInviting(true)
    setError("")
    setInviteSuccess("")
    try {
      const targetEmail = email.trim()
      await shareProject(project.id, targetEmail, "editor")
      setEmail("")
      setInviteSuccess(`Invitation dispatched to ${targetEmail} via email! Collaborators can accept via their email link or Invitations tab.`)
      await loadData()
      setTimeout(() => setInviteSuccess(""), 5000)
    } catch (err) {
      setError(err.response?.data?.error?.message || "Failed to send invitation")
    } finally {
      setInviting(false)
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
      <div className="bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] text-neutral-900 dark:text-white rounded-[12px] max-w-xl w-full p-6 shadow-2xl flex flex-col max-h-[85vh]">
        {/* Header */}
        <div className="flex items-start justify-between gap-4 mb-4 pb-3 border-b border-neutral-200 dark:border-[#2e2e2e]">
          <div>
            <div className="flex items-center gap-2">
              <span className="text-[10px] font-semibold px-2 py-0.5 rounded-[4px] bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30 uppercase tracking-wider flex items-center gap-1">
                <Share2 className="w-3 h-3" />
                Collaborate
              </span>
              <h2 className="text-lg font-bold text-neutral-900 dark:text-white tracking-tight">Share Workspace "{project.name}"</h2>
            </div>
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-1">
              Collaborate on this workspace and its maps with your team. Up to 10 members allowed. All collaborators have editor privileges.
            </p>
          </div>
          <button
            onClick={onClose}
            className="p-1 rounded-[6px] text-neutral-400 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#2a2a2a] transition-colors cursor-pointer"
          >
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Direct Workspace Link Bar */}
        <div className="mb-4 p-3 rounded-[8px] bg-neutral-50 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e]">
          <div className="flex items-center justify-between mb-1.5">
            <label className="text-[11px] font-semibold text-neutral-700 dark:text-neutral-300 uppercase tracking-wider">
              Shareable Workspace Link
            </label>
            <span className="text-[10px] text-neutral-400">Direct access URL</span>
          </div>
          <div className="flex items-center gap-2">
            <input
              type="text"
              readOnly
              value={shareUrl}
              onFocus={(e) => e.target.select()}
              className="flex-1 px-3 py-1.5 rounded-[6px] bg-white dark:bg-[#202020] border border-neutral-300 dark:border-[#2e2e2e] text-neutral-700 dark:text-neutral-300 font-mono text-xs focus:outline-none select-all"
            />
            <button
              type="button"
              onClick={handleCopyLink}
              className="inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-[6px] bg-[#3ecf8e]/10 hover:bg-[#3ecf8e]/20 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30 text-xs font-semibold transition-colors shrink-0 cursor-pointer shadow-2xs"
            >
              {copiedLink ? <Check className="w-3.5 h-3.5 text-[#3ecf8e]" /> : <Copy className="w-3.5 h-3.5" />}
              <span>{copiedLink ? "Copied!" : "Copy Link"}</span>
            </button>
          </div>
        </div>

        {/* Tab Selector */}
        <div className="flex items-center gap-2 mb-4">
          <button
            onClick={() => setActiveTab("members")}
            className={`px-3 py-1.5 rounded-[6px] text-xs font-medium transition-colors cursor-pointer ${
              activeTab === "members"
                ? "bg-[#3ecf8e] text-[#171717] font-semibold shadow-xs"
                : "bg-neutral-100 dark:bg-[#171717] text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white border border-neutral-200 dark:border-[#2e2e2e]"
            }`}
          >
            Collaborators ({members.length}/10)
          </button>
          <button
            onClick={() => setActiveTab("activities")}
            className={`px-3 py-1.5 rounded-[6px] text-xs font-medium transition-colors cursor-pointer ${
              activeTab === "activities"
                ? "bg-[#3ecf8e] text-[#171717] font-semibold shadow-xs"
                : "bg-neutral-100 dark:bg-[#171717] text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white border border-neutral-200 dark:border-[#2e2e2e]"
            }`}
          >
            Activity History ({activities.length})
          </button>
        </div>

        {error && (
          <div className="mb-4 p-3 rounded-[6px] bg-red-500/10 border border-red-500/20 text-red-500 dark:text-red-400 text-xs flex items-center gap-2">
            <span>{error}</span>
          </div>
        )}

        {inviteSuccess && (
          <div className="mb-4 p-3 rounded-[6px] bg-[#3ecf8e]/10 border border-[#3ecf8e]/30 text-[#24b47e] dark:text-[#3ecf8e] text-xs flex items-center gap-2 font-medium">
            <span>{inviteSuccess}</span>
          </div>
        )}

        {activeTab === "members" ? (
          <>
            {/* Invite Form */}
            <form onSubmit={handleInvite} className="mb-5 p-3 rounded-[8px] bg-neutral-50 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e]">
              <div className="flex items-center justify-between mb-2">
                <label className="text-[11px] font-semibold text-neutral-700 dark:text-neutral-300 uppercase tracking-wider">
                  Invite Collaborator by Email
                </label>
                <span className={`text-[11px] font-mono font-medium ${members.length >= 10 ? 'text-red-500 font-bold' : 'text-neutral-400'}`}>
                  {members.length} / 10 Members
                </span>
              </div>
              {members.length >= 10 ? (
                <div className="p-2.5 rounded-[6px] bg-amber-500/10 border border-amber-500/30 text-amber-600 dark:text-amber-400 text-xs">
                  Maximum member limit reached (10/10 members). Remove an existing collaborator to invite new members.
                </div>
              ) : (
                <div className="flex items-center gap-2">
                  <input
                    type="email"
                    required
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder="collaborator@organization.com"
                    className="flex-1 px-3 py-1.5 rounded-[6px] bg-white dark:bg-[#202020] border border-neutral-300 dark:border-[#2e2e2e] text-neutral-900 dark:text-white text-xs focus:outline-none focus:ring-1 focus:ring-[#3ecf8e] focus:border-[#3ecf8e]"
                  />
                  <button
                    type="submit"
                    disabled={inviting || !email.trim() || members.length >= 10}
                    className="px-3.5 py-1.5 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] text-xs font-semibold transition-colors disabled:opacity-50 shrink-0 cursor-pointer shadow-xs"
                  >
                    {inviting ? "Inviting..." : "Send Invite"}
                  </button>
                </div>
              )}
            </form>

            {/* Member List */}
            <div className="flex-1 overflow-y-auto space-y-2 pr-1">
              {loading ? (
                <div className="text-center py-8 text-xs text-neutral-500">Loading members...</div>
              ) : members.length === 0 ? (
                <div className="text-center py-8 text-xs text-neutral-500">No members found.</div>
              ) : (
                members.map((m) => {
                  const isOwner = m.role === "owner"
                  const isPending = m.invitation_status === "pending"

                  return (
                    <div
                      key={m.id}
                      className="p-3 rounded-[8px] bg-white dark:bg-[#1c1c1c] border border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-between gap-3 shadow-2xs"
                    >
                      <div className="flex items-center gap-2.5 overflow-hidden">
                        <div className="w-8 h-8 rounded-full bg-[#3ecf8e]/10 border border-[#3ecf8e]/30 flex items-center justify-center text-xs font-semibold text-[#24b47e] dark:text-[#3ecf8e] uppercase shrink-0">
                          {m.user_name?.charAt(0) || m.user_email?.charAt(0) || "U"}
                        </div>
                        <div className="truncate">
                          <div className="flex items-center gap-2">
                            <span className="text-xs font-medium text-neutral-900 dark:text-white truncate">
                              {m.user_name || "Invited User"}
                            </span>
                            {isOwner && (
                              <span className="text-[10px] px-1.5 py-0.2 rounded-[4px] bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30 font-semibold uppercase">
                                Owner
                              </span>
                            )}
                            {isPending && (
                              <span className="text-[10px] px-1.5 py-0.2 rounded-[4px] bg-amber-500/10 text-amber-500 border border-amber-500/30 font-medium animate-pulse uppercase">
                                Pending
                              </span>
                            )}
                          </div>
                          <div className="text-[11px] text-neutral-500 dark:text-neutral-400 truncate">{m.user_email}</div>
                        </div>
                      </div>

                      <div className="flex items-center gap-2 shrink-0">
                        {isOwner ? (
                          <span className="text-xs text-neutral-400 italic pr-2">Owner</span>
                        ) : (
                          <>
                            <span className="text-[10px] px-2 py-0.5 rounded-[4px] bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30 font-semibold uppercase">
                              Editor
                            </span>
                            <button
                              onClick={() => handleRemove(m)}
                              title={isPending ? "Cancel Invitation" : "Remove Collaborator"}
                              className="p-1 rounded-[4px] hover:bg-red-500/10 text-neutral-400 hover:text-red-500 transition-colors cursor-pointer"
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
              <div className="text-center py-10 text-xs text-neutral-500">No project activity recorded yet.</div>
            ) : (
              activities.map((act) => (
                <div
                  key={act.id}
                  className="p-3 rounded-[8px] bg-white dark:bg-[#1c1c1c] border border-neutral-200 dark:border-[#2e2e2e] flex items-start gap-3 shadow-2xs"
                >
                  <div className="w-2 h-2 rounded-full bg-[#3ecf8e] mt-1.5 shrink-0"></div>
                  <div className="flex-1">
                    <div className="text-xs text-neutral-800 dark:text-neutral-200">
                      <span className="font-semibold text-neutral-900 dark:text-white">{act.user_name}</span>: {act.details}
                    </div>
                    <div className="text-[10px] text-neutral-400 mt-1 font-mono">
                      {formatDateTime(act.timestamp || act.created_at)}
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>
        )}

        {/* Footer */}
        <div className="mt-4 pt-3 border-t border-neutral-200 dark:border-[#2e2e2e] flex justify-end">
          <button
            onClick={onClose}
            className="px-4 py-2 rounded-[6px] bg-neutral-100 dark:bg-[#2e2e2e] hover:bg-neutral-200 dark:hover:bg-[#383838] text-neutral-800 dark:text-neutral-200 text-xs font-medium transition-colors cursor-pointer"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  )
}
