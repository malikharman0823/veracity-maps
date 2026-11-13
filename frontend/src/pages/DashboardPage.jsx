import React, { useState, useEffect, useRef } from "react"
import { useNavigate, useSearchParams } from "react-router-dom"
import { useAuth } from "../context/AuthContext"
import {
  listMaps,
  createMap,
  updateMap,
  deleteMap,
  getMapInvitation,
  acceptMapInvitation,
} from "../api"
import {
  Users,
  Edit2,
  Trash2,
  Share2,
  LogOut,
  Shield,
  Map as MapIcon,
  Plus,
  ExternalLink,
  Layers,
  Clock,
  Check,
} from "lucide-react"
import ThemeToggle from "../components/ThemeToggle"
import ShareMapModal from "../components/ShareMapModal"
import { SidebarTrigger } from "@/components/ui/sidebar"
import { canDeleteSavedMap, resolveSavedMaps } from "../lib/savedMaps"

const formatDateTime = (dateVal, fallback = "----") => {
  if (!dateVal) return fallback
  try {
    let cleanVal = dateVal
    if (typeof cleanVal === "string") {
      cleanVal = cleanVal.replace(/\.(\d{3})\d+([+-].*|Z)?$/, (match, p1, p2) => "." + p1 + (p2 || ""))
      if (/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}/.test(cleanVal)) {
        cleanVal = cleanVal.replace(" ", "T")
      }
    }
    const d = new Date(cleanVal)
    if (isNaN(d.getTime()) || d.getFullYear() <= 1) return fallback
    return d.toLocaleString(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    })
  } catch {
    return fallback
  }
}

export default function DashboardPage() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()

  const validTabs = ["my-maps", "shared-with-me", "saved-maps", "recent", "settings"]
  const tabParam = searchParams.get("tab")
  const [activeTab, setActiveTab] = useState(
    validTabs.includes(tabParam) ? tabParam : "my-maps"
  )

  useEffect(() => {
    const nextTab = validTabs.includes(tabParam) ? tabParam : "my-maps"
    setActiveTab(nextTab)
  }, [tabParam])

  const [myMaps, setMyMaps] = useState([])
  const [sharedWithMe, setSharedWithMe] = useState([])
  const [savedMaps, setSavedMaps] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")

  // Toast notification
  const [toastMsg, setToastMsg] = useState("")
  const showToast = (msg) => {
    setToastMsg(msg)
    setTimeout(() => setToastMsg(""), 4000)
  }

  // Create Map modal
  const [showCreateMapModal, setShowCreateMapModal] = useState(false)
  const [newMapName, setNewMapName] = useState("")
  const [newMapDesc, setNewMapDesc] = useState("")
  const [creatingMap, setCreatingMap] = useState(false)

  // Edit Map modal
  const [editingMap, setEditingMap] = useState(null)
  const [editMapName, setEditMapName] = useState("")
  const [editMapDesc, setEditMapDesc] = useState("")
  const [updatingMap, setUpdatingMap] = useState(false)

  // Share Map modal
  const [sharingMap, setSharingMap] = useState(null)

  // Invitation account mismatch modal
  const [inviteMismatchModal, setInviteMismatchModal] = useState(null)
  const [switchingInviteAccount, setSwitchingInviteAccount] = useState(false)

  const acceptedTokensRef = useRef(new Set())
  const hasLoadedOnceRef = useRef(false)

  const fetchAllData = async (silent = false) => {
    try {
      if (!silent && !hasLoadedOnceRef.current) {
        setLoading(true)
      }
      setError("")
      const mapsData = await listMaps().catch(() => ({ my_maps: [], shared_with_me: [] }))

      const my = mapsData?.my_maps || []
      const shared = mapsData?.shared_with_me || []
      const saved = resolveSavedMaps(mapsData)
      setMyMaps(my)
      setSharedWithMe(shared)
      setSavedMaps(saved)
      hasLoadedOnceRef.current = true
      if (typeof window !== "undefined") {
        window.dispatchEvent(
          new CustomEvent("veracity:maps-updated", {
            detail: { myMaps: my, sharedWithMe: shared, savedMaps: saved },
          })
        )
      }
    } catch (err) {
      setError("Failed to load dashboard data. Please check connection.")
    } finally {
      setLoading(false)
    }
  }

  // Load dashboard maps immediately on mount or when user changes
  useEffect(() => {
    fetchAllData()
  }, [user?.id])

  // Handle invitation link verification & acceptance (StrictMode-safe)
  const inviteTokenParam = searchParams.get("accept_invite") || searchParams.get("token") || ""
  useEffect(() => {
    const token = inviteTokenParam
    if (!token || !user?.email) {
      return
    }
    const tokenKey = `${token}:${user.email.trim().toLowerCase()}`
    if (acceptedTokensRef.current.has(tokenKey)) {
      return
    }

    let cancelled = false
    ;(async () => {
      try {
        const inv = await getMapInvitation(token).catch(() => null)
        if (cancelled) return

        const currentEmail = (user.email || "").trim().toLowerCase()
        const invitedEmail = (inv?.email || "").trim().toLowerCase()

        if (invitedEmail && invitedEmail !== currentEmail) {
          acceptedTokensRef.current.add(tokenKey)
          setInviteMismatchModal({
            token,
            invitedEmail: inv.email,
            currentEmail: user.email,
            mapName: inv.map_name || "Shared Map",
            inviterName: inv.inviter_name || "",
          })
          return
        }

        const share = await acceptMapInvitation(token)
        if (cancelled) return
        acceptedTokensRef.current.add(tokenKey)

        showToast(`Map shared with you has been accepted!`)
        setSearchParams((prev) => {
          const next = new URLSearchParams(prev)
          next.delete("accept_invite")
          next.delete("token")
          return next
        }, { replace: true })
        setActiveTab("shared-with-me")
        await fetchAllData(true)
        if (share?.map_id) {
          navigate(`/maps/${share.map_id}/edit`)
        }
      } catch (err) {
        if (cancelled) return
        acceptedTokensRef.current.add(tokenKey)
        const errCode = err.response?.data?.error?.code
        const errDetails = err.response?.data?.error?.details || {}
        if (errCode === "INVITATION_EMAIL_MISMATCH") {
          setInviteMismatchModal({
            token,
            invitedEmail: errDetails.invited_email || "the invited email address",
            currentEmail: errDetails.current_email || user.email,
            mapName: errDetails.map_name || "Shared Map",
            inviterName: "",
          })
          return
        }
        const code = err.response?.status
        if (code === 404 || code === 409) {
          setActiveTab("shared-with-me")
        }
        console.warn("Notice accepting invitation token:", err.response?.data?.error?.message || err.message)
        fetchAllData(true)
      }
    })()

    return () => {
      cancelled = true
    }
  }, [inviteTokenParam, user?.email])

  const handleSwitchAccountForInvite = async () => {
    if (!inviteMismatchModal) return
    setSwitchingInviteAccount(true)
    const { invitedEmail, token } = inviteMismatchModal
    const redirectTarget = `/dashboard?accept_invite=${encodeURIComponent(token)}`
    try {
      await logout()
    } catch {}
    navigate(
      `/login?email=${encodeURIComponent(invitedEmail || "")}&redirect=${encodeURIComponent(redirectTarget)}`,
      { replace: true }
    )
  }

  const handleDismissInviteMismatch = () => {
    setInviteMismatchModal(null)
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      next.delete("accept_invite")
      next.delete("token")
      return next
    }, { replace: true })
  }

  const handleTabChange = (tab) => {
    setActiveTab(tab)
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      if (tab === "my-maps") {
        next.delete("tab")
      } else {
        next.set("tab", tab)
      }
      return next
    }, { replace: true })
  }

  const handleCreateMap = async (e) => {
    e.preventDefault()
    if (!newMapName.trim() || myMaps.length >= 10) return
    setCreatingMap(true)
    try {
      const created = await createMap(newMapName.trim(), newMapDesc.trim())
      setShowCreateMapModal(false)
      setNewMapName("")
      setNewMapDesc("")
      await fetchAllData()
      showToast(`Map "${created.name}" created!`)
      navigate(`/maps/${created.id}/edit`)
    } catch (err) {
      alert("Failed to create map: " + (err.response?.data?.error?.message || err.message))
    } finally {
      setCreatingMap(false)
    }
  }

  const handleUpdateMap = async (e) => {
    e.preventDefault()
    if (!editMapName.trim() || !editingMap) return
    setUpdatingMap(true)
    try {
      await updateMap(editingMap.id, editMapName.trim(), editMapDesc.trim())
      setEditingMap(null)
      await fetchAllData()
      showToast("Map updated successfully.")
    } catch (err) {
      alert("Failed to update map: " + (err.response?.data?.error?.message || err.message))
    } finally {
      setUpdatingMap(false)
    }
  }

  const handleDeleteMap = async (mapItem, e) => {
    e.stopPropagation()
    if (!window.confirm(`Are you sure you want to permanently delete map "${mapItem.name}"? This action cannot be undone.`)) {
      return
    }
    try {
      await deleteMap(mapItem.id)
      const nextMy = myMaps.filter((m) => m.id !== mapItem.id)
      const nextSaved = savedMaps.filter((m) => m.id !== mapItem.id)
      setMyMaps(nextMy)
      setSavedMaps(nextSaved)
      if (typeof window !== "undefined") {
        window.dispatchEvent(
          new CustomEvent("veracity:maps-updated", {
            detail: { myMaps: nextMy, sharedWithMe, savedMaps: nextSaved },
          })
        )
      }
      showToast(`Map "${mapItem.name}" deleted.`)
    } catch (err) {
      alert("Failed to delete map: " + (err.response?.data?.error?.message || err.message))
    }
  }

  // Recent Maps (sorted by updated_at)
  const combinedMaps = [...myMaps, ...sharedWithMe].sort((a, b) => {
    return new Date(b.updated_at || 0) - new Date(a.updated_at || 0)
  })

  return (
    <div className="min-h-screen bg-[#fafafa] dark:bg-[#171717] text-neutral-900 dark:text-neutral-100 font-sans transition-colors">
      {/* Toast */}
      {toastMsg && (
        <div className="fixed bottom-6 right-6 z-50 px-4 py-2.5 rounded-[8px] bg-neutral-900 dark:bg-white text-white dark:text-neutral-900 text-xs font-semibold shadow-lg flex items-center gap-2 animate-in fade-in slide-in-from-bottom-2 duration-200">
          <Check className="w-4 h-4 text-[#3ecf8e]" />
          <span>{toastMsg}</span>
        </div>
      )}

      {/* Top Header */}
      <header className="border-b border-neutral-200 dark:border-[#242424] bg-white/70 dark:bg-[#1c1c1c]/70 backdrop-blur-md sticky top-0 z-30">
        <div className="max-w-7xl mx-auto px-6 h-14 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <SidebarTrigger className="text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#282828] rounded-[6px] transition-colors" />
            <div className="w-8 h-8 rounded-[8px] bg-[#3ecf8e] text-[#171717] flex items-center justify-center font-bold text-sm shadow-xs">
              V
            </div>
            <span className="font-bold tracking-tight text-sm text-neutral-900 dark:text-white">
              VERACITY MAPS
            </span>
          </div>

          <div className="flex items-center gap-3">
            <ThemeToggle />
            <div className="h-4 w-px bg-neutral-200 dark:bg-[#2b2b2b]" />
            <div className="flex items-center gap-2">
              <div className="w-7 h-7 rounded-full bg-neutral-200 dark:bg-[#2b2b2b] text-neutral-700 dark:text-neutral-300 flex items-center justify-center text-xs font-semibold">
                {user?.name ? user.name[0].toUpperCase() : "U"}
              </div>
              <span className="text-xs font-medium text-neutral-700 dark:text-neutral-300 hidden sm:inline">
                {user?.name || user?.email}
              </span>
            </div>
            <button
              onClick={() => logout().then(() => navigate("/login"))}
              className="p-1.5 rounded-[6px] text-neutral-500 hover:text-red-500 hover:bg-neutral-100 dark:hover:bg-[#2b2b2b] transition-colors cursor-pointer"
              title="Sign out"
            >
              <LogOut className="w-4 h-4" />
            </button>
          </div>
        </div>
      </header>

      {/* Main Content */}
      <main className="max-w-7xl mx-auto px-6 py-8">
        {/* Profile / Personal Dashboard Title Banner */}
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 p-6 rounded-[12px] bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] shadow-2xs mb-8">
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-xl font-bold tracking-tight text-neutral-900 dark:text-white">
                Personal Dashboard
              </h1>
              <span className="px-2 py-0.5 rounded text-[10px] font-semibold bg-[#3ecf8e]/20 text-[#24b47e] dark:text-[#3ecf8e]">
                Independent Account
              </span>
            </div>
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-1">
              Welcome back, <strong className="text-neutral-700 dark:text-neutral-200">{user?.name || user?.email}</strong>. Manage your private maps, uploads, and individual map shares.
            </p>
          </div>

          <div className="flex items-center gap-4">
            {/* Map Slots Counter */}
            <div className="flex flex-col min-w-[130px]">
              <div className="flex items-center justify-between text-xs mb-1.5">
                <span className="font-semibold text-neutral-700 dark:text-neutral-300">My Maps</span>
                <span className={`font-bold ${myMaps.length >= 10 ? "text-red-500" : "text-[#3ecf8e]"}`}>
                  {myMaps.length} / 10
                </span>
              </div>
              <div className="w-full h-2 rounded-full bg-neutral-200 dark:bg-[#2b2b2b] overflow-hidden">
                <div
                  className={`h-full transition-all duration-300 ${
                    myMaps.length >= 10
                      ? "bg-red-500"
                      : myMaps.length >= 8
                      ? "bg-amber-500"
                      : "bg-[#3ecf8e]"
                  }`}
                  style={{ width: `${Math.min(100, (myMaps.length / 10) * 100)}%` }}
                />
              </div>
              <span className="text-[10px] text-neutral-400 mt-1">
                {10 - myMaps.length} map slot{10 - myMaps.length === 1 ? "" : "s"} available
              </span>
            </div>

            {/* Create Map Button */}
            <button
              onClick={() => {
                if (myMaps.length >= 10) {
                  alert("Account limit reached: Maximum 10 personal maps allowed.")
                  return
                }
                setShowCreateMapModal(true)
              }}
              disabled={myMaps.length >= 10}
              className={`inline-flex items-center gap-2 px-4 py-2.5 rounded-[6px] font-semibold text-xs transition-all shadow-xs shrink-0 ${
                myMaps.length >= 10
                  ? "bg-neutral-200 dark:bg-[#2e2e2e] text-neutral-400 cursor-not-allowed opacity-60"
                  : "bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] cursor-pointer"
              }`}
            >
              <Plus className="w-4 h-4" />
              New Map ({myMaps.length}/10)
            </button>
          </div>
        </div>

        {/* Navigation Tabs */}
        <div className="flex items-center gap-1.5 border-b border-neutral-200 dark:border-[#2e2e2e] pb-3 mb-6 overflow-x-auto">
          <button
            onClick={() => handleTabChange("my-maps")}
            className={`px-3.5 py-1.5 rounded-[6px] text-xs font-semibold transition-all shrink-0 cursor-pointer ${
              activeTab === "my-maps"
                ? "bg-[#3ecf8e] text-[#171717] shadow-xs"
                : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#242424]"
            }`}
          >
            My Maps ({myMaps.length}/10)
          </button>
          <button
            onClick={() => handleTabChange("shared-with-me")}
            className={`px-3.5 py-1.5 rounded-[6px] text-xs font-semibold transition-all shrink-0 cursor-pointer ${
              activeTab === "shared-with-me"
                ? "bg-[#3ecf8e] text-[#171717] shadow-xs"
                : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#242424]"
            }`}
          >
            Shared With Me ({sharedWithMe.length})
          </button>
          <button
            onClick={() => handleTabChange("saved-maps")}
            className={`px-3.5 py-1.5 rounded-[6px] text-xs font-semibold transition-all shrink-0 cursor-pointer ${
              activeTab === "saved-maps"
                ? "bg-[#3ecf8e] text-[#171717] shadow-xs"
                : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#242424]"
            }`}
          >
            Saved Maps ({savedMaps.length})
          </button>
          <button
            onClick={() => handleTabChange("recent")}
            className={`px-3.5 py-1.5 rounded-[6px] text-xs font-semibold transition-all shrink-0 cursor-pointer ${
              activeTab === "recent"
                ? "bg-[#3ecf8e] text-[#171717] shadow-xs"
                : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#242424]"
            }`}
          >
            Recent Maps ({combinedMaps.length})
          </button>
          <button
            onClick={() => handleTabChange("settings")}
            className={`px-3.5 py-1.5 rounded-[6px] text-xs font-semibold transition-all shrink-0 cursor-pointer ${
              activeTab === "settings"
                ? "bg-[#3ecf8e] text-[#171717] shadow-xs"
                : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#242424]"
            }`}
          >
            Profile & Security
          </button>
        </div>

        {/* Tab Content */}
        {loading ? (
          <div className="flex flex-col items-center justify-center py-24 text-neutral-500">
            <svg className="animate-spin h-8 w-8 text-[#3ecf8e] mb-3" fill="none" viewBox="0 0 24 24">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
            </svg>
            <span className="text-sm">Loading your maps and assets…</span>
          </div>
        ) : error ? (
          <div className="p-6 rounded-[12px] bg-red-500/10 border border-red-500/20 text-red-500 dark:text-red-400 text-center text-sm">
            {error}
          </div>
        ) : activeTab === "my-maps" ? (
          /* MY MAPS */
          <div>
            {myMaps.length === 0 ? (
              <div className="border border-dashed border-neutral-300 dark:border-[#2e2e2e] rounded-[12px] p-12 text-center flex flex-col items-center justify-center">
                <MapIcon className="w-12 h-12 text-neutral-400 mb-3 opacity-60" />
                <h4 className="text-base font-semibold text-neutral-900 dark:text-white">No personal maps yet</h4>
                <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-1 max-w-sm mb-4">
                  Create your first map to start georeferencing floorplans, CAD blueprints, and drone rasters.
                </p>
                <button
                  onClick={() => setShowCreateMapModal(true)}
                  className="inline-flex items-center gap-1.5 px-4 py-2 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-semibold text-xs cursor-pointer shadow-xs"
                >
                  <Plus className="w-4 h-4" />
                  Create First Map
                </button>
              </div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
                {myMaps.map((m) => {
                  const isLockedByMe = m.lock && m.lock.user_id === user?.id
                  const isLockedByOther = m.lock && m.lock.user_id !== user?.id

                  return (
                    <div
                      key={m.id}
                      className="p-5 rounded-[12px] bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] hover:border-[#3ecf8e]/50 transition-all flex flex-col justify-between shadow-2xs group relative"
                    >
                      <div>
                        {/* Status Tags */}
                        <div className="flex items-center justify-between gap-2 mb-2">
                          <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e]">
                            REV {m.revision_number || 1}
                          </span>

                          <div className="flex items-center gap-1.5">
                            {m.sharing_status === "shared" && (
                              <span className="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                                Shared
                              </span>
                            )}
                            {isLockedByOther && (
                              <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-amber-500/15 text-amber-500 border border-amber-500/30">
                                🔒 Locked
                              </span>
                            )}
                          </div>
                        </div>

                        {/* Title & Description */}
                        <h3 className="font-bold text-base text-neutral-900 dark:text-white group-hover:text-[#3ecf8e] transition-colors">
                          {m.name}
                        </h3>
                        <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-1 line-clamp-2">
                          {m.description || "No description provided."}
                        </p>
                      </div>

                      {/* Footer & Actions */}
                      <div className="mt-5 pt-4 border-t border-neutral-100 dark:border-[#2b2b2b] flex items-center justify-between text-[11px] text-neutral-400">
                        <div className="flex items-center gap-1">
                          <Clock className="w-3.5 h-3.5" />
                          <span>{formatDateTime(m.updated_at || m.created_at)}</span>
                        </div>

                        <div className="flex items-center gap-2">
                          <button
                            onClick={() => setSharingMap(m)}
                            className="p-1.5 rounded-[4px] hover:bg-neutral-100 dark:hover:bg-[#2b2b2b] text-neutral-500 hover:text-emerald-500 transition-colors"
                            title="Share Map"
                          >
                            <Share2 className="w-3.5 h-3.5" />
                          </button>
                          <button
                            onClick={() => {
                              setEditingMap(m)
                              setEditMapName(m.name)
                              setEditMapDesc(m.description || "")
                            }}
                            className="p-1.5 rounded-[4px] hover:bg-neutral-100 dark:hover:bg-[#2b2b2b] text-neutral-500 hover:text-neutral-800 dark:hover:text-white transition-colors"
                            title="Rename Map"
                          >
                            <Edit2 className="w-3.5 h-3.5" />
                          </button>
                          <button
                            onClick={(e) => handleDeleteMap(m, e)}
                            className="p-1.5 rounded-[4px] hover:bg-red-500/10 text-neutral-500 hover:text-red-500 transition-colors"
                            title="Delete Map"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                          <button
                            onClick={() => navigate(`/maps/${m.id}/edit`)}
                            className="inline-flex items-center gap-1 px-3 py-1.5 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-semibold text-xs cursor-pointer shadow-xs transition-colors ml-1"
                          >
                            <span>Open</span>
                            <ExternalLink className="w-3 h-3" />
                          </button>
                        </div>
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </div>
        ) : activeTab === "shared-with-me" ? (
          /* SHARED WITH ME */
          <div>
            {sharedWithMe.length === 0 ? (
              <div className="border border-dashed border-neutral-300 dark:border-[#2e2e2e] rounded-[12px] p-12 text-center flex flex-col items-center justify-center">
                <Users className="w-12 h-12 text-neutral-400 mb-3 opacity-60" />
                <h4 className="text-base font-semibold text-neutral-900 dark:text-white">No maps shared with you</h4>
                <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-1 max-w-sm">
                  When someone invites you to collaborate on a map or sends you a share link, it will appear here.
                </p>
              </div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
                {sharedWithMe.map((m) => {
                  const perm = m.permission || "viewer"
                  return (
                    <div
                      key={m.id}
                      className="p-5 rounded-[12px] bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] hover:border-[#3ecf8e]/50 transition-all flex flex-col justify-between shadow-2xs group"
                    >
                      <div>
                        <div className="flex items-center justify-between gap-2 mb-2">
                          <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e]">
                            REV {m.revision_number || 1}
                          </span>
                          <span className="px-2 py-0.5 rounded text-[10px] font-bold uppercase tracking-wider bg-emerald-500/15 text-emerald-400 border border-emerald-500/30">
                            Editor
                          </span>
                        </div>

                        <h3 className="font-bold text-base text-neutral-900 dark:text-white group-hover:text-[#3ecf8e] transition-colors">
                          {m.name}
                        </h3>
                        <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-1 line-clamp-2">
                          {m.description || "Shared map layout."}
                        </p>
                        <div className="mt-2 text-[11px] text-neutral-400">
                          Owner: <strong className="text-neutral-300">{m.shared_by_name || m.owner_user_name || m.owner_user_email || "Unknown owner"}</strong>
                        </div>
                      </div>

                      <div className="mt-5 pt-4 border-t border-neutral-100 dark:border-[#2b2b2b] flex items-center justify-between text-[11px] text-neutral-400">
                        <div className="flex items-center gap-1">
                          <Clock className="w-3.5 h-3.5" />
                          <span>{formatDateTime(m.updated_at || m.created_at)}</span>
                        </div>

                        <button
                          onClick={() => navigate(`/maps/${m.id}/edit`)}
                          className="inline-flex items-center gap-1 px-3 py-1.5 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-semibold text-xs cursor-pointer shadow-xs transition-colors"
                        >
                          <span>Open in Studio</span>
                          <ExternalLink className="w-3 h-3" />
                        </button>
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </div>
        ) : activeTab === "saved-maps" ? (
          /* PERSISTENT SAVED MAPS */
          <div className="rounded-[12px] bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] overflow-hidden shadow-2xs">
            <div className="p-4 border-b border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-between gap-4">
              <div>
                <h3 className="font-bold text-sm text-neutral-900 dark:text-white">
                  Saved Maps ({savedMaps.length})
                </h3>
                <p className="text-[11px] text-neutral-500 dark:text-neutral-400 mt-1">
                  These maps and their revision history remain available after sessions end and after you log out.
                </p>
              </div>
            </div>

            {savedMaps.length === 0 ? (
              <div className="p-10 text-center">
                <MapIcon className="w-10 h-10 text-neutral-400 mx-auto mb-3 opacity-60" />
                <div className="text-sm font-semibold text-neutral-900 dark:text-white">No saved maps yet</div>
                <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-1">
                  Save a workspace configuration to add a persistent map to this account.
                </p>
              </div>
            ) : (
              <div className="divide-y divide-neutral-100 dark:divide-[#2e2e2e]">
                {savedMaps.map((mapItem) => (
                  <div
                    key={mapItem.id}
                    className="p-4 flex items-center justify-between gap-4 hover:bg-neutral-50 dark:hover:bg-[#242424] transition-colors"
                  >
                    <div className="flex items-center gap-3 min-w-0">
                      <div className="p-2 rounded-lg bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] shrink-0">
                        <MapIcon className="w-4 h-4" />
                      </div>
                      <div className="min-w-0">
                        <div className="text-xs font-semibold text-neutral-900 dark:text-white truncate">
                          {mapItem.name}
                        </div>
                        <div className="text-[10px] text-neutral-400 mt-0.5">
                          Revision {mapItem.revision_number || 1} • Saved {formatDateTime(mapItem.updated_at || mapItem.created_at)}
                        </div>
                      </div>
                    </div>

                    <div className="flex items-center gap-2 shrink-0">
                      <button
                        onClick={() => navigate(`/maps/${mapItem.id}/edit`)}
                        className="inline-flex items-center gap-1 px-3 py-1.5 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-semibold text-xs cursor-pointer"
                      >
                        Open Map
                        <ExternalLink className="w-3 h-3" />
                      </button>
                      {canDeleteSavedMap(mapItem, user) && (
                        <button
                          onClick={(event) => handleDeleteMap(mapItem, event)}
                          className="p-1.5 rounded-[4px] hover:bg-red-500/10 text-neutral-500 hover:text-red-500 transition-colors"
                          title="Delete saved map"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        ) : activeTab === "recent" ? (
          /* RECENT MAPS */
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
            {combinedMaps.length === 0 ? (
              <div className="col-span-full border border-dashed border-neutral-300 dark:border-[#2e2e2e] rounded-[12px] p-12 text-center text-xs text-neutral-500">
                No recent maps available.
              </div>
            ) : (
              combinedMaps.map((m) => (
                <div
                  key={m.id}
                  className="p-5 rounded-[12px] bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] hover:border-[#3ecf8e]/50 transition-all flex flex-col justify-between shadow-2xs"
                >
                  <div>
                    <div className="flex items-center justify-between gap-2 mb-2">
                      <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e]">
                        REV {m.revision_number || 1}
                      </span>
                      <span className="text-[10px] text-neutral-400">
                        {m.owner_user_id === user?.id ? "Personal" : `Owner: ${m.shared_by_name || m.owner_user_name || m.owner_user_email || "Unknown owner"}`}
                      </span>
                    </div>

                    <h3 className="font-bold text-base text-neutral-900 dark:text-white">
                      {m.name}
                    </h3>
                    <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-1 line-clamp-2">
                      {m.description || "Map layout"}
                    </p>
                  </div>

                  <div className="mt-5 pt-4 border-t border-neutral-100 dark:border-[#2b2b2b] flex items-center justify-between text-[11px] text-neutral-400">
                    <span>{formatDateTime(m.updated_at || m.created_at)}</span>
                    <button
                      onClick={() => navigate(`/maps/${m.id}/edit`)}
                      className="inline-flex items-center gap-1 px-3 py-1.5 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-semibold text-xs cursor-pointer shadow-xs transition-colors"
                    >
                      <span>Open in Studio</span>
                      <ExternalLink className="w-3 h-3" />
                    </button>
                  </div>
                </div>
              ))
            )}
          </div>
        ) : (
          /* SETTINGS & PROFILE */
          <div className="max-w-2xl bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] rounded-[12px] p-6 shadow-2xs space-y-6">
            <h3 className="font-bold text-base text-neutral-900 dark:text-white">
              Account Information
            </h3>

            <div className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-neutral-500 dark:text-neutral-400 mb-1">
                  Full Name
                </label>
                <div className="px-3.5 py-2.5 rounded-[6px] bg-neutral-100 dark:bg-[#2b2b2b] text-sm text-neutral-900 dark:text-white font-medium">
                  {user?.name || "Not provided"}
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-neutral-500 dark:text-neutral-400 mb-1">
                  Email Address
                </label>
                <div className="px-3.5 py-2.5 rounded-[6px] bg-neutral-100 dark:bg-[#2b2b2b] text-sm text-neutral-900 dark:text-white font-medium flex items-center justify-between">
                  <span>{user?.email}</span>
                  <span className="text-[10px] font-bold text-emerald-500 uppercase px-2 py-0.5 rounded bg-emerald-500/10">
                    OTP Verified
                  </span>
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-neutral-500 dark:text-neutral-400 mb-1">
                  Architecture Model
                </label>
                <div className="px-3.5 py-2.5 rounded-[6px] bg-neutral-100 dark:bg-[#2b2b2b] text-xs text-neutral-600 dark:text-neutral-300 font-mono">
                  User → Personal Dashboard → My Maps → Share Map → Shared With Me
                </div>
              </div>
            </div>

            <div className="pt-4 border-t border-neutral-200 dark:border-[#2e2e2e] flex justify-between items-center">
              <span className="text-xs text-neutral-400">Veracity Maps v1.3</span>
              <button
                onClick={() => logout().then(() => navigate("/login"))}
                className="px-4 py-2 rounded-[6px] bg-red-500/10 hover:bg-red-500/20 text-red-500 text-xs font-semibold transition-colors cursor-pointer"
              >
                Sign Out
              </button>
            </div>
          </div>
        )}
      </main>

      {/* Create Map Modal */}
      {showCreateMapModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-xs p-4 animate-in fade-in duration-150">
          <div className="bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] rounded-[12px] w-full max-w-md p-6 shadow-xl">
            <h3 className="font-bold text-base text-neutral-900 dark:text-white mb-1">
              Create New Personal Map
            </h3>
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mb-4">
              Your map is stored privately under your account. You can invite collaborators or generate share links at any time.
            </p>

            <form onSubmit={handleCreateMap} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-neutral-700 dark:text-neutral-300 mb-1">
                  Map Name <span className="text-red-500">*</span>
                </label>
                <input
                  type="text"
                  value={newMapName}
                  onChange={(e) => setNewMapName(e.target.value)}
                  placeholder="e.g. Headquarters Ground Floor"
                  required
                  autoFocus
                  className="w-full px-3.5 py-2 rounded-[6px] bg-neutral-100 dark:bg-[#2b2b2b] border border-neutral-300 dark:border-[#383838] text-sm text-neutral-900 dark:text-white focus:outline-none focus:border-[#3ecf8e]"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-neutral-700 dark:text-neutral-300 mb-1">
                  Description (Optional)
                </label>
                <textarea
                  value={newMapDesc}
                  onChange={(e) => setNewMapDesc(e.target.value)}
                  placeholder="Brief summary of the area or floorplan…"
                  rows={3}
                  className="w-full px-3.5 py-2 rounded-[6px] bg-neutral-100 dark:bg-[#2b2b2b] border border-neutral-300 dark:border-[#383838] text-sm text-neutral-900 dark:text-white focus:outline-none focus:border-[#3ecf8e] resize-none"
                />
              </div>

              <div className="flex items-center justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setShowCreateMapModal(false)}
                  className="px-4 py-2 rounded-[6px] border border-neutral-300 dark:border-[#383838] text-xs font-semibold text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#2b2b2b] transition-colors"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={creatingMap || !newMapName.trim()}
                  className="px-4 py-2 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-semibold text-xs cursor-pointer shadow-xs disabled:opacity-50 transition-colors"
                >
                  {creatingMap ? "Creating…" : "Create & Open Studio"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Edit Map Modal */}
      {editingMap && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-xs p-4 animate-in fade-in duration-150">
          <div className="bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] rounded-[12px] w-full max-w-md p-6 shadow-xl">
            <h3 className="font-bold text-base text-neutral-900 dark:text-white mb-1">
              Edit Map Details
            </h3>

            <form onSubmit={handleUpdateMap} className="space-y-4 mt-4">
              <div>
                <label className="block text-xs font-semibold text-neutral-700 dark:text-neutral-300 mb-1">
                  Map Name <span className="text-red-500">*</span>
                </label>
                <input
                  type="text"
                  value={editMapName}
                  onChange={(e) => setEditMapName(e.target.value)}
                  required
                  autoFocus
                  className="w-full px-3.5 py-2 rounded-[6px] bg-neutral-100 dark:bg-[#2b2b2b] border border-neutral-300 dark:border-[#383838] text-sm text-neutral-900 dark:text-white focus:outline-none focus:border-[#3ecf8e]"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-neutral-700 dark:text-neutral-300 mb-1">
                  Description
                </label>
                <textarea
                  value={editMapDesc}
                  onChange={(e) => setEditMapDesc(e.target.value)}
                  rows={3}
                  className="w-full px-3.5 py-2 rounded-[6px] bg-neutral-100 dark:bg-[#2b2b2b] border border-neutral-300 dark:border-[#383838] text-sm text-neutral-900 dark:text-white focus:outline-none focus:border-[#3ecf8e] resize-none"
                />
              </div>

              <div className="flex items-center justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setEditingMap(null)}
                  className="px-4 py-2 rounded-[6px] border border-neutral-300 dark:border-[#383838] text-xs font-semibold text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#2b2b2b] transition-colors"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={updatingMap || !editMapName.trim()}
                  className="px-4 py-2 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-semibold text-xs cursor-pointer shadow-xs disabled:opacity-50 transition-colors"
                >
                  {updatingMap ? "Saving…" : "Save Changes"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Share Map Modal */}
      {sharingMap && (
        <ShareMapModal
          map={sharingMap}
          isOpen={Boolean(sharingMap)}
          onClose={() => setSharingMap(null)}
          onShareUpdated={fetchAllData}
        />
      )}

      {/* Invitation Email Mismatch Modal */}
      {inviteMismatchModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-xs p-4 animate-in fade-in duration-150">
          <div className="bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] rounded-[12px] w-full max-w-md p-6 shadow-2xl">
            <div className="flex items-center gap-3 mb-3">
              <div className="w-10 h-10 rounded-[8px] bg-amber-500/15 border border-amber-500/30 text-amber-500 flex items-center justify-center shrink-0">
                <Shield className="w-5 h-5" />
              </div>
              <div>
                <h3 className="font-bold text-base text-neutral-900 dark:text-white">
                  Different Account Required
                </h3>
                <p className="text-xs text-neutral-500 dark:text-neutral-400">
                  Invitation email verification check
                </p>
              </div>
            </div>

            <div className="p-3.5 rounded-[8px] bg-neutral-100 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] text-xs space-y-2 mb-5">
              <p className="text-neutral-700 dark:text-neutral-300 leading-relaxed">
                This invitation for <span className="font-semibold text-neutral-900 dark:text-white">{inviteMismatchModal.mapName}</span> was sent to:
              </p>
              <div className="px-2.5 py-1.5 rounded-[6px] bg-[#3ecf8e]/10 border border-[#3ecf8e]/30 text-[#24b47e] dark:text-[#3ecf8e] font-mono font-semibold break-all">
                {inviteMismatchModal.invitedEmail}
              </div>
              <p className="text-neutral-500 dark:text-neutral-400 pt-1">
                You are currently signed in as <span className="font-mono font-medium text-neutral-800 dark:text-neutral-200">{inviteMismatchModal.currentEmail}</span>. To accept this shared map and open its files, please sign in with the invited email address.
              </p>
            </div>

            <div className="flex flex-col sm:flex-row items-center justify-end gap-2">
              <button
                type="button"
                onClick={handleDismissInviteMismatch}
                disabled={switchingInviteAccount}
                className="w-full sm:w-auto px-4 py-2 rounded-[6px] border border-neutral-300 dark:border-[#383838] text-xs font-semibold text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#2b2b2b] transition-colors cursor-pointer"
              >
                Stay as {inviteMismatchModal.currentEmail}
              </button>
              <button
                type="button"
                onClick={handleSwitchAccountForInvite}
                disabled={switchingInviteAccount}
                className="w-full sm:w-auto px-4 py-2 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-semibold text-xs cursor-pointer shadow-xs disabled:opacity-50 transition-colors"
              >
                {switchingInviteAccount
                  ? "Switching account…"
                  : `Sign in as ${inviteMismatchModal.invitedEmail}`}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
