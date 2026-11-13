import React, { useCallback, useEffect, useState } from "react"
import { Share2, Copy, Check, Trash2, Mail, Link as LinkIcon, Shield, Users, Clock } from "lucide-react"
import {
  inviteToMap,
  listMapShares,
  revokeMapShare,
  createShareLink,
  listShareLinks,
  revokeShareLink,
} from "../api"
import { getShareMapId } from "../lib/mapSharing"

export default function ShareMapModal({ map, isOpen, onClose, onShareUpdated }) {
  const [activeTab, setActiveTab] = useState("email") // "email" | "links" | "members"
  const [shares, setShares] = useState([])
  const [invitations, setInvitations] = useState([])
  const [links, setLinks] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [success, setSuccess] = useState("")

  // Email form
  const [email, setEmail] = useState("")
  const [inviting, setInviting] = useState(false)

  // Share link form
  const [linkPublic, setLinkPublic] = useState(true)
  const [expiresInDays, setExpiresInDays] = useState(7)
  const [creatingLink, setCreatingLink] = useState(false)
  const [copiedLink, setCopiedLink] = useState("")
  const mapId = getShareMapId(map, isOpen)

  const loadData = useCallback(async () => {
    if (!mapId) return

    try {
      setLoading(true)
      setError("")
      const [sharesData, linksData] = await Promise.all([
        listMapShares(mapId).catch(() => ({ shares: [], invitations: [] })),
        listShareLinks(mapId).catch(() => []),
      ])
      setShares(sharesData.shares || [])
      setInvitations(sharesData.invitations || [])
      setLinks(linksData || [])
    } catch (err) {
      setError(err.response?.data?.error?.message || "Failed to load sharing details")
    } finally {
      setLoading(false)
    }
  }, [mapId])

  useEffect(() => {
    if (!mapId) return
    loadData()
  }, [loadData, mapId])

  if (!mapId) return null

  const totalMembers = 1 + shares.length + invitations.length // Owner + active shares + pending invitations
  const isCapReached = totalMembers >= 10

  const handleInvite = async (e) => {
    e.preventDefault()
    if (!email.trim() || isCapReached) return

    setInviting(true)
    setError("")
    setSuccess("")
    try {
      const cleanEmail = email.trim()
      await inviteToMap(mapId, cleanEmail, "editor")
      setEmail("")
      setSuccess(`Invitation dispatched to ${cleanEmail}! Recipient will receive an email to accept and edit the map.`)
      await loadData()
      if (onShareUpdated) onShareUpdated()
      setTimeout(() => setSuccess(""), 5000)
    } catch (err) {
      setError(err.response?.data?.error?.message || "Failed to send invitation")
    } finally {
      setInviting(false)
    }
  }

  const handleRevokeShare = async (shareId) => {
    if (!window.confirm("Are you sure you want to revoke access for this collaborator?")) return
    try {
      await revokeMapShare(mapId, shareId)
      await loadData()
      if (onShareUpdated) onShareUpdated()
    } catch (err) {
      setError(err.response?.data?.error?.message || "Failed to revoke access")
    }
  }

  const handleCreateLink = async (e) => {
    e.preventDefault()
    setCreatingLink(true)
    setError("")
    setSuccess("")
    try {
      const res = await createShareLink(mapId, {
        permission: "editor",
        isPublic: linkPublic,
        expiresInDays: Number(expiresInDays),
      })
      const fullUrl = res?.share_url || `${window.location.origin}/share/${res?.token}`
      setCopiedLink(fullUrl)
      navigator.clipboard.writeText(fullUrl)
      setSuccess("Share link created with Editor privileges and copied to clipboard!")
      await loadData()
      setTimeout(() => setSuccess(""), 5000)
    } catch (err) {
      setError(err.response?.data?.error?.message || "Failed to create share link")
    } finally {
      setCreatingLink(false)
    }
  }

  const handleRevokeLink = async (linkId) => {
    if (!window.confirm("Are you sure you want to deactivate this share link?")) return
    try {
      await revokeShareLink(mapId, linkId)
      await loadData()
    } catch (err) {
      setError(err.response?.data?.error?.message || "Failed to revoke share link")
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4 animate-in fade-in duration-200">
      <div className="bg-[#18181b] border border-[#27272a] rounded-2xl w-full max-w-xl overflow-hidden shadow-2xl text-slate-100">
        {/* Header */}
        <div className="p-6 border-b border-[#27272a] flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="p-2.5 rounded-xl bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
              <Share2 className="w-5 h-5" />
            </div>
            <div>
              <h2 className="text-lg font-semibold tracking-tight text-white flex items-center gap-2">
                Share Map: <span className="text-emerald-400 font-bold">{map.name}</span>
              </h2>
              <p className="text-xs text-slate-400 mt-0.5">
                Invite collaborators via email or generate secure shareable links
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="text-slate-400 hover:text-white p-2 rounded-lg hover:bg-[#27272a] transition-colors"
          >
            ✕
          </button>
        </div>

        {/* Tab Controls */}
        <div className="flex border-b border-[#27272a] px-6 bg-[#121215]">
          <button
            onClick={() => setActiveTab("email")}
            className={`py-3 px-4 text-xs font-medium border-b-2 transition-colors flex items-center gap-2 ${
              activeTab === "email"
                ? "border-emerald-500 text-emerald-400"
                : "border-transparent text-slate-400 hover:text-slate-200"
            }`}
          >
            <Mail className="w-3.5 h-3.5" />
            Invite via Email
          </button>
          <button
            onClick={() => setActiveTab("links")}
            className={`py-3 px-4 text-xs font-medium border-b-2 transition-colors flex items-center gap-2 ${
              activeTab === "links"
                ? "border-emerald-500 text-emerald-400"
                : "border-transparent text-slate-400 hover:text-slate-200"
            }`}
          >
            <LinkIcon className="w-3.5 h-3.5" />
            Shareable Links ({links.length})
          </button>
          <button
            onClick={() => setActiveTab("members")}
            className={`py-3 px-4 text-xs font-medium border-b-2 transition-colors flex items-center gap-2 ${
              activeTab === "members"
                ? "border-emerald-500 text-emerald-400"
                : "border-transparent text-slate-400 hover:text-slate-200"
            }`}
          >
            <Users className="w-3.5 h-3.5" />
            Collaborators ({totalMembers}/10)
          </button>
        </div>

        {/* Alerts */}
        {error && (
          <div className="mx-6 mt-4 p-3 rounded-lg bg-red-500/10 border border-red-500/20 text-xs text-red-400">
            {error}
          </div>
        )}
        {success && (
          <div className="mx-6 mt-4 p-3 rounded-lg bg-emerald-500/10 border border-emerald-500/20 text-xs text-emerald-400 flex items-center gap-2">
            <Check className="w-4 h-4 shrink-0" />
            {success}
          </div>
        )}

        {/* Tab Content */}
        <div className="p-6 max-h-[60vh] overflow-y-auto">
          {activeTab === "email" && (
            <div className="space-y-4">
              {isCapReached && (
                <div className="p-3 rounded-xl bg-amber-500/10 border border-amber-500/20 text-xs text-amber-400 flex items-center gap-2">
                  <Shield className="w-4 h-4 shrink-0 text-amber-400" />
                  <span>Maximum limit reached (10/10 members). Remove a collaborator to invite new members.</span>
                </div>
              )}

              <form onSubmit={handleInvite} className="space-y-4">
                <div>
                  <label className="block text-xs font-medium text-slate-300 mb-1.5">
                    Collaborator Email
                  </label>
                  <input
                    type="email"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder="colleague@example.com"
                    required
                    disabled={isCapReached || inviting}
                    className="w-full px-3.5 py-2.5 rounded-xl bg-[#202024] border border-[#2e2e33] text-sm text-white placeholder-slate-500 focus:outline-none focus:border-emerald-500 transition-colors disabled:opacity-50"
                  />
                </div>

                {/* Editor Privileges Notice (Role selection removed: all shared members are Editors) */}
                <div className="p-3.5 rounded-xl bg-[#202024] border border-[#2e2e33]">
                  <div className="flex items-center gap-2 text-xs font-semibold text-emerald-400">
                    <span className="size-2 rounded-full bg-emerald-400"></span>
                    Privilege Level: Editor
                  </div>
                  <p className="text-[11px] text-slate-400 mt-1 leading-relaxed">
                    All shared members have full editing privileges (view, edit layers, and save configurations). Only you (the owner) can delete the shared project.
                  </p>
                </div>

                <div className="flex items-center justify-between pt-2">
                  <div className="text-xs text-slate-400">
                    Cap: <span className={`font-semibold ${isCapReached ? "text-amber-400" : "text-slate-200"}`}>{totalMembers} of 10</span> members
                  </div>
                  <button
                    type="submit"
                    disabled={inviting || isCapReached || !email.trim()}
                    className="px-5 py-2 rounded-xl bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-semibold shadow-lg shadow-emerald-600/20 transition-all disabled:opacity-50"
                  >
                    {inviting ? "Sending…" : "Send Email Invitation"}
                  </button>
                </div>
              </form>

              {/* Pending Invitations section */}
              {invitations.length > 0 && (
                <div className="mt-6 pt-4 border-t border-[#27272a]">
                  <h4 className="text-xs font-semibold text-slate-300 uppercase tracking-wider mb-2.5">
                    Pending Email Invitations ({invitations.length})
                  </h4>
                  <div className="space-y-2">
                    {invitations.map((inv) => (
                      <div
                        key={inv.id}
                        className="flex items-center justify-between p-3 rounded-xl bg-[#202024] border border-[#2e2e33]"
                      >
                        <div className="flex items-center gap-2.5">
                          <Clock className="w-4 h-4 text-amber-400" />
                          <div>
                            <div className="text-xs font-medium text-white">{inv.email}</div>
                            <div className="text-[10px] text-slate-400">
                              Role: <span className="text-emerald-400 font-semibold uppercase">Editor</span> • Pending acceptance
                            </div>
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          )}

          {activeTab === "links" && (
            <div className="space-y-5">
              <form onSubmit={handleCreateLink} className="p-4 rounded-xl bg-[#202024] border border-[#2e2e33] space-y-3">
                <div className="flex items-center justify-between">
                  <h3 className="text-xs font-semibold text-white">Generate New Share Link</h3>
                  <span className="text-[10px] text-emerald-400 font-semibold uppercase tracking-wider bg-emerald-500/10 px-2 py-0.5 rounded border border-emerald-500/20">
                    Editor Access
                  </span>
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block text-[10px] text-slate-400 mb-1">Expiration</label>
                    <select
                      value={expiresInDays}
                      onChange={(e) => setExpiresInDays(e.target.value)}
                      className="w-full px-2.5 py-1.5 rounded-lg bg-[#18181b] border border-[#333] text-xs text-white"
                    >
                      <option value="1">1 Day</option>
                      <option value="7">7 Days</option>
                      <option value="30">30 Days</option>
                      <option value="0">Never expires</option>
                    </select>
                  </div>
                  <div>
                    <label className="block text-[10px] text-slate-400 mb-1">Visibility</label>
                    <select
                      value={linkPublic ? "public" : "private"}
                      onChange={(e) => setLinkPublic(e.target.value === "public")}
                      className="w-full px-2.5 py-1.5 rounded-lg bg-[#18181b] border border-[#333] text-xs text-white"
                    >
                      <option value="public">Anyone with link</option>
                      <option value="private">Registered users</option>
                    </select>
                  </div>
                </div>

                <p className="text-[11px] text-slate-400">
                  Shared links grant Editor privileges to open and edit the map in Studio. Only the owner can delete the map.
                </p>

                <button
                  type="submit"
                  disabled={creatingLink}
                  className="w-full mt-2 py-2 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-semibold transition-colors disabled:opacity-50"
                >
                  {creatingLink ? "Generating…" : "Create & Copy Share Link"}
                </button>
              </form>

              {/* Existing links */}
              <div>
                <h4 className="text-xs font-semibold text-slate-300 uppercase tracking-wider mb-2.5">
                  Active Share Links ({links.length})
                </h4>
                {links.length === 0 ? (
                  <p className="text-xs text-slate-500 py-2">No active shareable links.</p>
                ) : (
                  <div className="space-y-2">
                    {links.map((link) => {
                      const linkUrl = `${window.location.origin}/share/${link.token || link.id}`
                      return (
                        <div
                          key={link.id}
                          className="flex items-center justify-between p-3 rounded-xl bg-[#202024] border border-[#2e2e33]"
                        >
                          <div className="truncate pr-3">
                            <div className="flex items-center gap-2">
                              <span className="text-xs font-semibold text-emerald-400 uppercase tracking-wider">
                                Editor
                              </span>
                              <span className="text-[10px] text-slate-400">
                                {link.is_public ? "Public" : "Auth-only"} •{" "}
                                {link.expires_at ? `Expires ${new Date(link.expires_at).toLocaleDateString()}` : "No expiry"}
                              </span>
                            </div>
                            <div className="text-[11px] text-slate-400 font-mono truncate mt-0.5">
                              {linkUrl}
                            </div>
                          </div>
                          <div className="flex items-center gap-1.5 shrink-0">
                            <button
                              onClick={() => {
                                navigator.clipboard.writeText(linkUrl)
                                setCopiedLink(link.id)
                                setTimeout(() => setCopiedLink(""), 2500)
                              }}
                              className="p-1.5 rounded-lg bg-[#2e2e33] hover:bg-[#38383e] text-slate-200 transition-colors"
                              title="Copy link"
                            >
                              {copiedLink === link.id ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                            </button>
                            <button
                              onClick={() => handleRevokeLink(link.id)}
                              className="p-1.5 rounded-lg bg-red-500/10 hover:bg-red-500/20 text-red-400 transition-colors"
                              title="Revoke link"
                            >
                              <Trash2 className="w-3.5 h-3.5" />
                            </button>
                          </div>
                        </div>
                      )
                    })}
                  </div>
                )}
              </div>
            </div>
          )}

          {activeTab === "members" && (
            <div className="space-y-3">
              {/* Owner card */}
              <div className="flex items-center justify-between p-3 rounded-xl bg-[#202024] border border-emerald-500/20">
                <div className="flex items-center gap-2.5">
                  <Shield className="w-4 h-4 text-emerald-400" />
                  <div>
                    <div className="text-xs font-semibold text-white">
                      {map.owner_user_name || "Owner"}
                    </div>
                    <div className="text-[10px] text-slate-400">{map.owner_user_email || "Map Owner"}</div>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <span className="px-2 py-0.5 rounded text-[10px] font-semibold bg-emerald-500/20 text-emerald-300">
                    Owner (Can Delete)
                  </span>
                </div>
              </div>

              {/* Shared users */}
              {shares.length === 0 ? (
                <div className="p-4 text-center text-xs text-slate-500">
                  No other collaborators yet. Invite colleagues using their email address.
                </div>
              ) : (
                shares.map((s) => (
                  <div
                    key={s.id}
                    className="flex items-center justify-between p-3 rounded-xl bg-[#202024] border border-[#2e2e33]"
                  >
                    <div>
                      <div className="text-xs font-medium text-white">{s.user_name || s.user_email}</div>
                      <div className="text-[10px] text-slate-400">{s.user_email}</div>
                    </div>

                    <div className="flex items-center gap-2">
                      <span className="px-2 py-0.5 rounded text-[10px] font-semibold bg-emerald-500/15 text-emerald-400 border border-emerald-500/30">
                        Editor
                      </span>

                      <button
                        onClick={() => handleRevokeShare(s.id)}
                        className="p-1.5 rounded-lg bg-red-500/10 hover:bg-red-500/20 text-red-400 transition-colors"
                        title="Revoke access"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </div>
                ))
              )}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="p-4 bg-[#121215] border-t border-[#27272a] flex justify-end">
          <button
            onClick={onClose}
            className="px-4 py-2 rounded-xl bg-[#27272a] hover:bg-[#323238] text-white text-xs font-medium transition-colors"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  )
}
