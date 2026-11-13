import React, { useState, useEffect, useMemo } from "react"
import { useNavigate, Link } from "react-router-dom"
import { useAuth } from "../context/AuthContext"
import {
  getProfileDashboard,
  updateProfileName,
  deleteProfileFile,
  respondToInvitation,
  switchSession,
  getActiveSessionId,
  deleteSession,
} from "../api"
import TopHeader from "../components/TopHeader"
import HoldButton from "../components/HoldButton"
import {
  User,
  Mail,
  Calendar,
  ShieldCheck,
  FolderKanban,
  MapPin,
  CheckCircle2,
  Clock,
  Trash2,
  ExternalLink,
  Download,
  Edit3,
  Compass,
  Search,
  Users,
  FileImage,
  ArrowRight,
  HardDrive,
  Cpu,
  Layers,
  History,
  AlertCircle,
  RefreshCw,
} from "lucide-react"

export const formatDateTime = (dateVal, fallback = "----") => {
  if (!dateVal) return fallback
  try {
    let cleanVal = dateVal
    if (typeof cleanVal === "string") {
      // Trim nanoseconds to milliseconds while keeping timezone intact
      cleanVal = cleanVal.replace(/\.(\d{3})\d+([+-].*|Z)?$/, (match, p1, p2) => "." + p1 + (p2 || ""))
      // Convert SQL datetime format with space to ISO format with T
      if (/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}/.test(cleanVal)) {
        cleanVal = cleanVal.replace(" ", "T")
      }
    }
    const d = new Date(cleanVal)
    if (isNaN(d.getTime()) || d.getFullYear() <= 1) return fallback
    const formatted = d.toLocaleString(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    })
    return formatted === "Invalid Date" ? fallback : formatted
  } catch {
    return fallback
  }
}

export default function ProfilePage() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()

  const [dashboardData, setDashboardData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [activeTab, setActiveTab] = useState("files") // "files" | "sessions" | "projects" | "shared" | "invitations"

  // Inline name editing state
  const [isEditingName, setIsEditingName] = useState(false)
  const [newName, setNewName] = useState("")
  const [savingName, setSavingName] = useState(false)

  // File search & filter
  const [fileSearch, setFileSearch] = useState("")
  const [fileFilter, setFileFilter] = useState("all") // "all" | "georeferenced" | "unreferenced"

  // Delete modal/action state
  const [deletingFileId, setDeletingFileId] = useState(null)
  const [actionMessage, setActionMessage] = useState("")

  const currentSessionId = getActiveSessionId()

  const fetchProfileData = async () => {
    try {
      setLoading(true)
      setError("")
      const data = await getProfileDashboard()
      setDashboardData(data)
      if (data?.user?.full_name || data?.user?.name) {
        setNewName(data.user.full_name || data.user.name)
      }
    } catch (err) {
      console.error("Failed to load profile dashboard:", err)
      setError(err.response?.data?.error?.message || "Failed to load profile dashboard.")
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchProfileData()
  }, [])

  const handleUpdateName = async (e) => {
    e.preventDefault()
    if (!newName.trim()) return
    try {
      setSavingName(true)
      await updateProfileName(newName.trim())
      setIsEditingName(false)
      setActionMessage("Profile name updated successfully.")
      setTimeout(() => setActionMessage(""), 4000)
      await fetchProfileData()
    } catch (err) {
      alert("Failed to update name: " + (err.response?.data?.error?.message || err.message))
    } finally {
      setSavingName(false)
    }
  }

  const handleDeleteFile = async (fileId, filename) => {
    if (!window.confirm(`Are you sure you want to delete "${filename}"? This action cannot be undone.`)) {
      return
    }
    try {
      setDeletingFileId(fileId)
      await deleteProfileFile(fileId)
      setActionMessage(`Dataset "${filename}" was removed.`)
      setTimeout(() => setActionMessage(""), 4000)
      await fetchProfileData()
    } catch (err) {
      alert("Failed to delete file: " + (err.response?.data?.error?.message || err.message))
    } finally {
      setDeletingFileId(null)
    }
  }

  const handleDeleteSession = async (sessionId) => {
    try {
      await deleteSession(sessionId)
      setActionMessage("Workspace session removed.")
      setTimeout(() => setActionMessage(""), 4000)
      await fetchProfileData()
    } catch (err) {
      console.warn("Failed to delete session:", err)
    }
  }

  const handleInvitationResponse = async (invitationId, accept) => {
    try {
      await respondToInvitation(invitationId, accept)
      setActionMessage(accept ? "Invitation accepted!" : "Invitation declined.")
      setTimeout(() => setActionMessage(""), 4000)
      await fetchProfileData()
    } catch (err) {
      alert("Failed to respond to invitation: " + (err.response?.data?.error?.message || err.message))
    }
  }

  const formatFileSize = (bytes) => {
    if (!bytes || bytes <= 0) return "0 B"
    const units = ["B", "KB", "MB", "GB"]
    const i = Math.floor(Math.log(bytes) / Math.log(1024))
    return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`
  }

  const filteredFiles = useMemo(() => {
    const list = dashboardData?.files || []
    return list.filter((f) => {
      const matchSearch =
        !fileSearch ||
        f.filename.toLowerCase().includes(fileSearch.toLowerCase()) ||
        (f.project_name && f.project_name.toLowerCase().includes(fileSearch.toLowerCase()))
      if (!matchSearch) return false

      if (fileFilter === "georeferenced") {
        return f.georeference_status === "completed"
      }
      if (fileFilter === "unreferenced") {
        return f.georeference_status !== "completed"
      }
      return true
    })
  }, [dashboardData?.files, fileSearch, fileFilter])

  const profileUser = dashboardData?.user || user
  const stats = dashboardData?.stats || {
    total_projects: 0,
    shared_projects: 0,
    total_files: 0,
    georeferenced_files: 0,
    pending_invitations: 0,
    total_sessions: 0,
    total_memory: "0 B",
  }

  const sessionsList = dashboardData?.sessions || []

  if (loading && !dashboardData) {
    return (
      <div className="min-h-screen bg-neutral-50 dark:bg-[#141414] text-neutral-900 dark:text-neutral-100 flex flex-col font-sans transition-colors">
        <TopHeader
          breadcrumbs={[
            { label: "Dashboard", path: "/dashboard" },
            { label: "User Profile" },
          ]}
        />
        <main className="flex-1 max-w-7xl w-full mx-auto p-4 sm:p-6 lg:p-8 flex items-center justify-center">
          <div className="flex flex-col items-center gap-3">
            <div className="size-8 border-2 border-[#3ecf8e] border-t-transparent rounded-full animate-spin"></div>
            <span className="text-xs text-neutral-500 dark:text-neutral-400">Loading profile data…</span>
          </div>
        </main>
      </div>
    )
  }

  if (error && !dashboardData) {
    return (
      <div className="min-h-screen bg-neutral-50 dark:bg-[#141414] text-neutral-900 dark:text-neutral-100 flex flex-col font-sans transition-colors">
        <TopHeader
          breadcrumbs={[
            { label: "Dashboard", path: "/dashboard" },
            { label: "User Profile" },
          ]}
        />
        <main className="flex-1 max-w-7xl w-full mx-auto p-4 sm:p-6 lg:p-8 flex items-center justify-center">
          <div className="p-6 rounded-xl bg-white dark:bg-[#1c1c1c] border border-red-500/30 max-w-md w-full text-center space-y-3">
            <AlertCircle className="size-8 text-red-500 mx-auto" />
            <h3 className="text-base font-semibold text-neutral-900 dark:text-white">Unable to load profile</h3>
            <p className="text-xs text-neutral-500 dark:text-neutral-400">{error}</p>
            <button
              onClick={fetchProfileData}
              className="px-4 py-2 text-xs font-semibold rounded-lg bg-[#3ecf8e] text-[#141414] hover:bg-[#24b47e] transition-colors cursor-pointer"
            >
              Retry
            </button>
          </div>
        </main>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-neutral-50 dark:bg-[#141414] text-neutral-900 dark:text-neutral-100 flex flex-col font-sans transition-colors">
      <TopHeader
        breadcrumbs={[
          { label: "Dashboard", path: "/dashboard" },
          { label: "User Profile" },
        ]}
      />

      <main className="flex-1 max-w-7xl w-full mx-auto p-4 sm:p-6 lg:p-8 space-y-8">
        {/* Banner / Notification */}
        {actionMessage && (
          <div className="bg-[#3ecf8e]/10 border border-[#3ecf8e]/30 text-[#10b981] dark:text-[#3ecf8e] text-sm px-4 py-3 rounded-lg flex items-center justify-between shadow-xs">
            <span className="flex items-center gap-2">
              <CheckCircle2 className="size-4 shrink-0" />
              {actionMessage}
            </span>
            <button
              onClick={() => setActionMessage("")}
              className="text-xs hover:underline cursor-pointer"
            >
              Dismiss
            </button>
          </div>
        )}

        {error && (
          <div className="bg-red-500/10 border border-red-500/30 text-red-600 dark:text-red-400 text-sm px-4 py-3 rounded-lg flex items-center gap-2">
            <AlertCircle className="size-4 shrink-0" />
            <span>{error}</span>
          </div>
        )}

        {/* User Profile Header Card */}
        <div className="bg-white dark:bg-[#1c1c1c] border border-neutral-200 dark:border-[#2e2e2e] rounded-xl p-6 sm:p-8 shadow-xs relative overflow-hidden">
          <div className="absolute top-0 right-0 w-64 h-64 bg-gradient-to-bl from-[#3ecf8e]/10 to-transparent pointer-events-none rounded-tr-xl" />

          <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-6 relative z-10">
            <div className="flex items-center gap-5">
              {/* Avatar */}
              <div className="relative group">
                <div className="size-20 sm:size-24 rounded-full bg-gradient-to-tr from-[#24b47e] to-[#3ecf8e] text-[#141414] font-black text-2xl sm:text-3xl flex items-center justify-center border-4 border-white dark:border-[#1c1c1c] shadow-md ring-2 ring-[#3ecf8e]/20">
                  {profileUser?.full_name?.charAt(0) || profileUser?.name?.charAt(0) || "U"}
                </div>
                <div className="absolute bottom-0 right-0 size-6 bg-[#10b981] border-2 border-white dark:border-[#1c1c1c] rounded-full flex items-center justify-center text-white" title="Active">
                  <ShieldCheck className="size-3.5" />
                </div>
              </div>

              {/* User Details */}
              <div className="space-y-1">
                {isEditingName ? (
                  <form onSubmit={handleUpdateName} className="flex items-center gap-2">
                    <input
                      type="text"
                      value={newName}
                      onChange={(e) => setNewName(e.target.value)}
                      className="px-3 py-1.5 text-base font-semibold bg-neutral-100 dark:bg-[#282828] border border-[#3ecf8e] rounded-md outline-hidden text-neutral-900 dark:text-white"
                      placeholder="Your full name"
                      autoFocus
                    />
                    <button
                      type="submit"
                      disabled={savingName}
                      className="px-3 py-1.5 text-xs font-semibold bg-[#3ecf8e] text-[#141414] hover:bg-[#24b47e] rounded-md transition-colors cursor-pointer"
                    >
                      {savingName ? "Saving..." : "Save"}
                    </button>
                    <button
                      type="button"
                      onClick={() => setIsEditingName(false)}
                      className="px-2 py-1.5 text-xs text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300"
                    >
                      Cancel
                    </button>
                  </form>
                ) : (
                  <div className="flex items-center gap-2.5">
                    <h1 className="text-xl sm:text-2xl font-bold tracking-tight text-neutral-900 dark:text-white">
                      {profileUser?.full_name || profileUser?.name || "Veracity User"}
                    </h1>
                    <button
                      onClick={() => {
                        setNewName(profileUser?.full_name || profileUser?.name || "")
                        setIsEditingName(true)
                      }}
                      className="text-neutral-400 hover:text-[#3ecf8e] p-1 rounded-md hover:bg-neutral-100 dark:hover:bg-[#282828] transition-colors cursor-pointer"
                      title="Edit Full Name"
                    >
                      <Edit3 className="size-3.5" />
                    </button>
                  </div>
                )}

                <div className="flex flex-wrap items-center gap-3 text-xs text-neutral-500 dark:text-neutral-400">
                  <span className="flex items-center gap-1.5">
                    <Mail className="size-3.5 text-neutral-400" />
                    {profileUser?.email || "No email on file"}
                  </span>
                  <span className="size-1 rounded-full bg-neutral-300 dark:bg-neutral-600" />
                  <span className="flex items-center gap-1.5 text-[#10b981] dark:text-[#3ecf8e] font-medium">
                    <ShieldCheck className="size-3.5" />
                    Verified Account
                  </span>
                  {profileUser?.created_at && (
                    <>
                      <span className="size-1 rounded-full bg-neutral-300 dark:bg-neutral-600" />
                      <span className="flex items-center gap-1.5">
                        <Calendar className="size-3.5 text-neutral-400" />
                        Joined {formatDateTime(profileUser.created_at)}
                      </span>
                    </>
                  )}
                </div>
              </div>
            </div>

            {/* Quick Actions */}
            <div className="flex items-center gap-3 w-full sm:w-auto">
              <Link
                to="/"
                className="flex-1 sm:flex-initial inline-flex items-center justify-center gap-2 px-4 py-2 text-xs font-semibold rounded-lg bg-[#3ecf8e] text-[#141414] hover:bg-[#24b47e] transition-colors shadow-xs"
              >
                <Compass className="size-3.5" />
                Launch Workspace
              </Link>
              <button
                type="button"
                onClick={logout}
                className="flex-1 sm:flex-initial inline-flex items-center justify-center gap-2 px-4 py-2 text-xs font-medium rounded-lg border border-neutral-200 dark:border-[#2e2e2e] hover:bg-neutral-100 dark:hover:bg-[#282828] text-neutral-700 dark:text-neutral-300 transition-colors"
              >
                Sign Out
              </button>
            </div>
          </div>
        </div>

        {/* Metrics Grid */}
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
          {/* Uploaded Files */}
          <div
            onClick={() => setActiveTab("files")}
            className={`p-5 rounded-xl border transition-all cursor-pointer ${
              activeTab === "files"
                ? "bg-[#3ecf8e]/10 border-[#3ecf8e]/40 shadow-xs"
                : "bg-white dark:bg-[#1c1c1c] border-neutral-200 dark:border-[#2e2e2e] hover:border-neutral-300 dark:hover:border-neutral-700"
            }`}
          >
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">
                Uploaded Files
              </span>
              <div className="size-8 rounded-lg bg-[#3ecf8e]/15 text-[#10b981] dark:text-[#3ecf8e] flex items-center justify-center">
                <FileImage className="size-4" />
              </div>
            </div>
            <div className="mt-3 text-2xl font-bold text-neutral-900 dark:text-white">
              {stats.total_files}
            </div>
            <div className="mt-1 text-[11px] text-neutral-500 flex items-center gap-1">
              <span>{stats.georeferenced_files} georeferenced</span>
            </div>
          </div>

          {/* Total File Memory */}
          <div
            onClick={() => setActiveTab("files")}
            className="p-5 rounded-xl border bg-white dark:bg-[#1c1c1c] border-neutral-200 dark:border-[#2e2e2e] hover:border-neutral-300 dark:hover:border-neutral-700 transition-all cursor-pointer"
          >
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">
                File Memory
              </span>
              <div className="size-8 rounded-lg bg-emerald-500/15 text-emerald-500 flex items-center justify-center">
                <HardDrive className="size-4" />
              </div>
            </div>
            <div className="mt-3 text-2xl font-bold font-mono text-neutral-900 dark:text-white">
              {stats.total_memory || "0 B"}
            </div>
            <div className="mt-1 text-[11px] text-neutral-500">
              Active disk memory
            </div>
          </div>

          {/* Owned Projects */}
          <div
            onClick={() => setActiveTab("projects")}
            className={`p-5 rounded-xl border transition-all cursor-pointer ${
              activeTab === "projects"
                ? "bg-[#3ecf8e]/10 border-[#3ecf8e]/40 shadow-xs"
                : "bg-white dark:bg-[#1c1c1c] border-neutral-200 dark:border-[#2e2e2e] hover:border-neutral-300 dark:hover:border-neutral-700"
            }`}
          >
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">
                Personal Maps
              </span>
              <div className="size-8 rounded-lg bg-blue-500/15 text-blue-500 flex items-center justify-center">
                <FolderKanban className="size-4" />
              </div>
            </div>
            <div className="mt-3 text-2xl font-bold text-neutral-900 dark:text-white">
              {stats.total_projects}
            </div>
            <div className="mt-1 text-[11px] text-neutral-500">
              Personal (Only You)
            </div>
          </div>

          {/* Shared & Invitations */}
          <div
            onClick={() => setActiveTab(stats.pending_invitations > 0 ? "invitations" : "shared")}
            className={`p-5 rounded-xl border transition-all cursor-pointer ${
              activeTab === "shared" || activeTab === "invitations"
                ? "bg-[#3ecf8e]/10 border-[#3ecf8e]/40 shadow-xs"
                : "bg-white dark:bg-[#1c1c1c] border-neutral-200 dark:border-[#2e2e2e] hover:border-neutral-300 dark:hover:border-neutral-700"
            }`}
          >
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">
                Shared Maps
              </span>
              <div className="size-8 rounded-lg bg-purple-500/15 text-purple-500 flex items-center justify-center">
                <Users className="size-4" />
              </div>
            </div>
            <div className="mt-3 text-2xl font-bold text-neutral-900 dark:text-white flex items-center gap-2">
              {stats.shared_projects}
              {stats.pending_invitations > 0 && (
                <span className="text-xs font-bold px-2 py-0.5 rounded-full bg-amber-500/15 text-amber-500 border border-amber-500/30">
                  +{stats.pending_invitations} invites
                </span>
              )}
            </div>
            <div className="mt-1 text-[11px] text-neutral-500">
              {stats.pending_invitations > 0
                ? `${stats.pending_invitations} awaiting response`
                : "Collaborative Mapspaces"}
            </div>
          </div>
        </div>

        {/* Navigation Tabs */}
        <div className="border-b border-neutral-200 dark:border-[#2e2e2e]">
          <nav className="flex space-x-6 overflow-x-auto">
            <button
              onClick={() => setActiveTab("files")}
              className={`pb-3 text-sm font-semibold border-b-2 transition-colors cursor-pointer flex items-center gap-2 shrink-0 ${
                activeTab === "files"
                  ? "border-[#3ecf8e] text-[#10b981] dark:text-[#3ecf8e]"
                  : "border-transparent text-neutral-500 hover:text-neutral-800 dark:hover:text-neutral-200"
              }`}
            >
              <FileImage className="size-4" />
              Uploaded Files & Maps ({dashboardData?.files?.length || 0})
            </button>
            <button
              onClick={() => setActiveTab("projects")}
              className={`pb-3 text-sm font-semibold border-b-2 transition-colors cursor-pointer flex items-center gap-2 shrink-0 ${
                activeTab === "projects"
                  ? "border-[#3ecf8e] text-[#10b981] dark:text-[#3ecf8e]"
                  : "border-transparent text-neutral-500 hover:text-neutral-800 dark:hover:text-neutral-200"
              }`}
            >
              <FolderKanban className="size-4" />
              My Maps ({dashboardData?.projects?.length || 0})
            </button>
            <button
              onClick={() => setActiveTab("shared")}
              className={`pb-3 text-sm font-semibold border-b-2 transition-colors cursor-pointer flex items-center gap-2 shrink-0 ${
                activeTab === "shared"
                  ? "border-[#3ecf8e] text-[#10b981] dark:text-[#3ecf8e]"
                  : "border-transparent text-neutral-500 hover:text-neutral-800 dark:hover:text-neutral-200"
              }`}
            >
              <Users className="size-4" />
              Shared Maps ({dashboardData?.shared_projects?.length || 0})
            </button>
            {stats.pending_invitations > 0 && (
              <button
                onClick={() => setActiveTab("invitations")}
                className={`pb-3 text-sm font-semibold border-b-2 transition-colors cursor-pointer flex items-center gap-2 shrink-0 ${
                  activeTab === "invitations"
                    ? "border-[#3ecf8e] text-[#10b981] dark:text-[#3ecf8e]"
                    : "border-transparent text-neutral-500 hover:text-neutral-800 dark:hover:text-neutral-200"
                }`}
              >
                <Clock className="size-4 text-amber-500" />
                Invitations ({stats.pending_invitations})
              </button>
            )}
          </nav>
        </div>

        {/* Tab 1: Uploaded Files & Datasets */}
        {activeTab === "files" && (
          <div className="space-y-4">
            {/* Toolbar */}
            <div className="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3">
              <div className="relative flex-1 max-w-md">
                <Search className="size-4 absolute left-3 top-1/2 -translate-y-1/2 text-neutral-400" />
                <input
                  type="text"
                  value={fileSearch}
                  onChange={(e) => setFileSearch(e.target.value)}
                  placeholder="Search files by name or project..."
                  className="w-full pl-9 pr-4 py-2 text-xs bg-white dark:bg-[#1c1c1c] border border-neutral-200 dark:border-[#2e2e2e] rounded-lg outline-hidden text-neutral-900 dark:text-white focus:border-[#3ecf8e]"
                />
              </div>

              <div className="flex items-center gap-2 flex-wrap">
                <div className="flex items-center bg-white dark:bg-[#1c1c1c] border border-neutral-200 dark:border-[#2e2e2e] rounded-lg p-0.5 text-xs">
                  <button
                    onClick={() => setFileFilter("all")}
                    className={`px-3 py-1.5 rounded-md font-medium transition-colors cursor-pointer ${
                      fileFilter === "all"
                        ? "bg-[#3ecf8e] text-[#141414] font-semibold"
                        : "text-neutral-500 hover:text-neutral-800 dark:hover:text-neutral-200"
                    }`}
                  >
                    All ({dashboardData?.files?.length || 0})
                  </button>
                  <button
                    onClick={() => setFileFilter("georeferenced")}
                    className={`px-3 py-1.5 rounded-md font-medium transition-colors cursor-pointer ${
                      fileFilter === "georeferenced"
                        ? "bg-[#3ecf8e] text-[#141414] font-semibold"
                        : "text-neutral-500 hover:text-neutral-800 dark:hover:text-neutral-200"
                    }`}
                  >
                    Georeferenced ({stats.georeferenced_files})
                  </button>
                  <button
                    onClick={() => setFileFilter("unreferenced")}
                    className={`px-3 py-1.5 rounded-md font-medium transition-colors cursor-pointer ${
                      fileFilter === "unreferenced"
                        ? "bg-[#3ecf8e] text-[#141414] font-semibold"
                        : "text-neutral-500 hover:text-neutral-800 dark:hover:text-neutral-200"
                    }`}
                  >
                    Raw ({Math.max(0, stats.total_files - stats.georeferenced_files)})
                  </button>
                </div>

                <Link
                  to="/"
                  className="inline-flex items-center gap-1.5 px-3 py-2 text-xs font-semibold rounded-lg bg-[#3ecf8e] text-[#141414] hover:bg-[#24b47e] transition-colors shadow-xs"
                >
                  <Compass className="size-3.5" />
                  Upload Map
                </Link>
              </div>
            </div>

            {/* Files List / Table */}
            {filteredFiles.length === 0 ? (
              <div className="bg-white dark:bg-[#1c1c1c] border border-neutral-200 dark:border-[#2e2e2e] rounded-xl p-12 text-center space-y-3">
                <div className="size-12 rounded-full bg-neutral-100 dark:bg-[#282828] text-neutral-400 mx-auto flex items-center justify-center">
                  <HardDrive className="size-6" />
                </div>
                <h3 className="text-sm font-semibold text-neutral-900 dark:text-white">
                  {fileSearch ? "No files match your query" : "No uploaded files yet"}
                </h3>
                <p className="text-xs text-neutral-500 max-w-sm mx-auto">
                  {fileSearch
                    ? "Try adjusting your search terms or filters."
                    : "Upload historical aerial imagery, geological surveys, or drone maps to start analyzing."}
                </p>
                {!fileSearch && (
                  <Link
                    to="/"
                    className="inline-flex items-center gap-2 px-4 py-2 text-xs font-semibold rounded-lg bg-[#3ecf8e] text-[#141414] hover:bg-[#24b47e] transition-colors mt-2"
                  >
                    <Compass className="size-3.5" />
                    Upload First Dataset
                  </Link>
                )}
              </div>
            ) : (
              <div className="bg-white dark:bg-[#1c1c1c] border border-neutral-200 dark:border-[#2e2e2e] rounded-xl overflow-hidden shadow-xs divide-y divide-neutral-100 dark:divide-[#282828]">
                {filteredFiles.map((file) => {
                  const isGeoreferenced = file.georeference_status === "completed"
                  return (
                    <div
                      key={file.id}
                      className="p-4 sm:p-5 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 hover:bg-neutral-50/70 dark:hover:bg-[#242424]/50 transition-colors"
                    >
                      {/* Left: Icon & Details */}
                      <div className="flex items-start gap-3.5 min-w-0">
                        <div className="size-11 rounded-lg bg-neutral-100 dark:bg-[#282828] border border-neutral-200 dark:border-[#383838] flex items-center justify-center text-neutral-500 dark:text-neutral-400 shrink-0 font-bold text-[10px] uppercase">
                          {file.format || "MAP"}
                        </div>

                        <div className="space-y-1 min-w-0">
                          <div className="flex items-center gap-2 flex-wrap">
                            <span className="text-sm font-semibold text-neutral-900 dark:text-white truncate">
                              {file.filename}
                            </span>
                            {isGeoreferenced ? (
                              <span className="inline-flex items-center gap-1 text-[10px] font-semibold px-2 py-0.5 rounded-full bg-[#10b981]/15 text-[#10b981] dark:text-[#3ecf8e] border border-[#10b981]/30">
                                <CheckCircle2 className="size-3" />
                                Georeferenced
                              </span>
                            ) : (
                              <span className="inline-flex items-center gap-1 text-[10px] font-medium px-2 py-0.5 rounded-full bg-neutral-100 dark:bg-[#282828] text-neutral-500 border border-neutral-200 dark:border-[#333]">
                                <Clock className="size-3" />
                                Unreferenced
                              </span>
                            )}
                            {file.project_name && (
                              <span className="inline-flex items-center gap-1 text-[10px] font-medium px-2 py-0.5 rounded-md bg-blue-500/10 text-blue-500 border border-blue-500/20">
                                <FolderKanban className="size-3" />
                                {file.project_name}
                              </span>
                            )}
                          </div>

                          <div className="flex flex-wrap items-center gap-2.5 text-xs text-neutral-500 dark:text-neutral-400">
                            {/* Memory Badge */}
                            <span className="font-mono font-semibold text-neutral-800 dark:text-neutral-200 bg-neutral-100 dark:bg-[#282828] px-2 py-0.5 rounded border border-neutral-200 dark:border-[#383838]">
                              Memory: {file.memory_size || formatFileSize(file.file_size)}
                            </span>

                            {file.ram_estimate && (
                              <span className="font-mono text-[11px] text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded border border-emerald-500/20 flex items-center gap-1">
                                <Cpu className="size-3" />
                                RAM: ~{file.ram_estimate}
                              </span>
                            )}

                            {file.width && file.height && (
                              <>
                                <span className="size-1 rounded-full bg-neutral-300 dark:bg-neutral-600" />
                                <span>
                                  {file.width} × {file.height} px
                                </span>
                              </>
                            )}
                            {file.crs && (
                              <>
                                <span className="size-1 rounded-full bg-neutral-300 dark:bg-neutral-600" />
                                <span className="font-mono text-[11px]">{file.crs}</span>
                              </>
                            )}
                            <span className="size-1 rounded-full bg-neutral-300 dark:bg-neutral-600" />
                            <span className="text-neutral-400">
                              Uploaded {formatDateTime(file.created_at)}
                            </span>
                          </div>
                        </div>
                      </div>

                      {/* Right: Actions */}
                      <div className="flex items-center gap-2 w-full sm:w-auto justify-end shrink-0 pt-2 sm:pt-0 border-t sm:border-t-0 border-neutral-100 dark:border-[#282828]">
                        <Link
                          to={`/datasets/${file.id}/workspace`}
                          className="inline-flex items-center gap-1 px-3 py-1.5 text-xs font-semibold rounded-md bg-[#3ecf8e] text-[#141414] hover:bg-[#24b47e] transition-colors shadow-xs"
                          title="Open in Workspace"
                        >
                          <Compass className="size-3.5" />
                          Open
                        </Link>

                        {!isGeoreferenced && (
                          <Link
                            to={`/datasets/${file.id}/georeference`}
                            className="inline-flex items-center gap-1 px-3 py-1.5 text-xs font-medium rounded-md border border-[#3ecf8e]/30 text-[#10b981] dark:text-[#3ecf8e] hover:bg-[#3ecf8e]/10 transition-colors"
                            title="Start Georeferencing"
                          >
                            <MapPin className="size-3.5" />
                            Georeference
                          </Link>
                        )}

                        {file.download_url && (
                          <a
                            href={file.download_url}
                            download={file.filename}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="p-1.5 rounded-md border border-neutral-200 dark:border-[#2e2e2e] text-neutral-500 hover:text-neutral-800 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#282828] transition-colors"
                            title="Download Raw File"
                          >
                            <Download className="size-3.5" />
                          </a>
                        )}

                        <button
                          onClick={() => handleDeleteFile(file.id, file.filename)}
                          disabled={deletingFileId === file.id}
                          className="p-1.5 rounded-md border border-neutral-200 dark:border-[#2e2e2e] text-neutral-500 hover:text-red-500 hover:border-red-500/30 hover:bg-red-500/10 transition-colors cursor-pointer"
                          title="Delete Dataset"
                        >
                          <Trash2 className="size-3.5" />
                        </button>
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </div>
        )}

        {/* Tab 2: Owned Maps */}
        {activeTab === "projects" && (
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <h3 className="text-sm font-semibold text-neutral-900 dark:text-white">
                  My Maps ({dashboardData?.projects?.length || 0})
                </h3>
                <p className="text-xs text-neutral-500">
                  Maps created and owned by your account
                </p>
              </div>
              <Link
                to="/dashboard"
                className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium border border-neutral-200 dark:border-[#2e2e2e] rounded-md hover:bg-neutral-100 dark:hover:bg-[#282828] transition-colors"
              >
                Maps Dashboard
                <ExternalLink className="size-3" />
              </Link>
            </div>

            {(dashboardData?.projects || []).length === 0 ? (
              <div className="bg-white dark:bg-[#1c1c1c] border border-neutral-200 dark:border-[#2e2e2e] rounded-xl p-10 text-center space-y-3">
                <FolderKanban className="size-8 text-neutral-400 mx-auto" />
                <h4 className="text-sm font-semibold text-neutral-900 dark:text-white">No maps created yet</h4>
                <p className="text-xs text-neutral-500 max-w-sm mx-auto">
                  Create a map to bundle floor plans, aerial imagery, and georeferenced layers.
                </p>
                <Link
                  to="/dashboard"
                  className="inline-flex items-center gap-2 px-4 py-2 text-xs font-semibold rounded-lg bg-[#3ecf8e] text-[#141414] hover:bg-[#24b47e] transition-colors"
                >
                  Create Map
                </Link>
              </div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {(dashboardData?.projects || []).map((proj) => (
                  <div
                    key={proj.id}
                    className="bg-white dark:bg-[#1c1c1c] border border-neutral-200 dark:border-[#2e2e2e] rounded-xl p-5 space-y-3 hover:border-neutral-300 dark:hover:border-neutral-700 transition-all flex flex-col justify-between"
                  >
                    <div className="space-y-1.5">
                      <div className="flex items-center justify-between gap-2">
                        <span className="text-xs font-bold uppercase tracking-wider text-[#10b981] dark:text-[#3ecf8e]">
                          Owner
                        </span>
                        <span className="text-[11px] text-neutral-400">
                          Created: {formatDateTime(proj.created_at)}
                        </span>
                      </div>
                      <h4 className="text-base font-bold text-neutral-900 dark:text-white truncate">
                        {proj.name}
                      </h4>
                      <p className="text-xs text-neutral-500 line-clamp-2">
                        {proj.description || "No description provided."}
                      </p>
                    </div>

                    <div className="pt-3 border-t border-neutral-100 dark:border-[#282828] flex items-center justify-between">
                      <span className="text-xs text-neutral-500">
                        {proj.dataset_ids?.length || (proj.primary_dataset_id ? 1 : 0)} datasets
                      </span>
                      <Link
                        to={`/dashboard`}
                        className="inline-flex items-center gap-1 text-xs font-semibold text-[#10b981] dark:text-[#3ecf8e] hover:underline"
                      >
                        Manage
                        <ArrowRight className="size-3.5" />
                      </Link>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {/* Tab 4: Shared Maps */}
        {activeTab === "shared" && (
          <div className="space-y-4">
            <div>
              <h3 className="text-sm font-semibold text-neutral-900 dark:text-white">
                Maps Shared With You ({dashboardData?.shared_projects?.length || 0})
              </h3>
              <p className="text-xs text-neutral-500">
                Maps where colleagues invited you as an editor
              </p>
            </div>

            {(dashboardData?.shared_projects || []).length === 0 ? (
              <div className="bg-white dark:bg-[#1c1c1c] border border-neutral-200 dark:border-[#2e2e2e] rounded-xl p-10 text-center space-y-3">
                <Users className="size-8 text-neutral-400 mx-auto" />
                <h4 className="text-sm font-semibold text-neutral-900 dark:text-white">No shared workspaces</h4>
                <p className="text-xs text-neutral-500 max-w-sm mx-auto">
                  When team members invite you to collaborate, their projects will appear here.
                </p>
              </div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {(dashboardData?.shared_projects || []).map((sp, idx) => (
                  <div
                    key={sp.project?.id || idx}
                    className="bg-white dark:bg-[#1c1c1c] border border-neutral-200 dark:border-[#2e2e2e] rounded-xl p-5 space-y-3 flex flex-col justify-between"
                  >
                    <div className="space-y-1.5">
                      <div className="flex items-center justify-between gap-2">
                        <span className="text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded-full bg-purple-500/10 text-purple-500 border border-purple-500/20">
                          {sp.role || "collaborator"}
                        </span>
                        <span className="text-[11px] text-neutral-400">
                          Shared {formatDateTime(sp.shared_date)}
                        </span>
                      </div>
                      <h4 className="text-base font-bold text-neutral-900 dark:text-white truncate">
                        {sp.project?.name || "Shared Workspace"}
                      </h4>
                      <p className="text-xs text-neutral-500">
                        Owner: {sp.owner_name || sp.owner_email}
                      </p>
                    </div>

                    <div className="pt-3 border-t border-neutral-100 dark:border-[#282828] flex items-center justify-between">
                      <span className="text-xs text-neutral-400 font-mono text-[10px]">
                        {sp.project?.dataset_ids?.length || 0} datasets
                      </span>
                      <Link
                        to={`/dashboard?tab=shared`}
                        className="inline-flex items-center gap-1 text-xs font-semibold text-[#10b981] dark:text-[#3ecf8e] hover:underline"
                      >
                        Open Workspace
                        <ArrowRight className="size-3.5" />
                      </Link>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {/* Tab 5: Pending Invitations */}
        {activeTab === "invitations" && (
          <div className="space-y-4">
            <div>
              <h3 className="text-sm font-semibold text-neutral-900 dark:text-white">
                Pending Invitations ({dashboardData?.pending_invitations?.length || 0})
              </h3>
              <p className="text-xs text-neutral-500">
                Invitations sent to your email address ({profileUser?.email})
              </p>
            </div>

            {(dashboardData?.pending_invitations || []).length === 0 ? (
              <div className="bg-white dark:bg-[#1c1c1c] border border-neutral-200 dark:border-[#2e2e2e] rounded-xl p-10 text-center space-y-2">
                <CheckCircle2 className="size-8 text-[#10b981] mx-auto" />
                <h4 className="text-sm font-semibold text-neutral-900 dark:text-white">All caught up!</h4>
                <p className="text-xs text-neutral-500">You have no pending collaboration invitations.</p>
              </div>
            ) : (
              <div className="space-y-3">
                {dashboardData.pending_invitations.map((inv) => (
                  <div
                    key={inv.invitation_id}
                    className="bg-white dark:bg-[#1c1c1c] border border-neutral-200 dark:border-[#2e2e2e] rounded-xl p-5 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4"
                  >
                    <div className="space-y-1">
                      <div className="flex items-center gap-2">
                        <h4 className="text-sm font-bold text-neutral-900 dark:text-white">
                          {inv.project_name}
                        </h4>
                        <span className="text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded-full bg-amber-500/10 text-amber-500 border border-amber-500/20">
                          {inv.role}
                        </span>
                      </div>
                      <p className="text-xs text-neutral-500">
                        Invited by <strong className="text-neutral-700 dark:text-neutral-300">{inv.inviter_name || "A collaborator"}</strong> on {formatDateTime(inv.created_at)}
                      </p>
                    </div>

                    <div className="flex items-center gap-2">
                      <button
                        onClick={() => handleInvitationResponse(inv.invitation_id, true)}
                        className="px-4 py-1.5 text-xs font-semibold rounded-md bg-[#3ecf8e] text-[#141414] hover:bg-[#24b47e] transition-colors cursor-pointer"
                      >
                        Accept
                      </button>
                      <button
                        onClick={() => handleInvitationResponse(inv.invitation_id, false)}
                        className="px-3 py-1.5 text-xs font-medium rounded-md border border-neutral-200 dark:border-[#2e2e2e] text-neutral-500 hover:text-red-500 hover:bg-neutral-100 dark:hover:bg-[#282828] transition-colors cursor-pointer"
                      >
                        Decline
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
      </main>
    </div>
  )
}
