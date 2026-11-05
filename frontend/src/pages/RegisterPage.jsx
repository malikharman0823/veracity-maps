import React, { useState } from "react"
import { Link, useNavigate } from "react-router-dom"
import { useAuth } from "../context/AuthContext"
import ThemeToggle from "../components/ThemeToggle"

export default function RegisterPage() {
  const { register } = useAuth()
  const navigate = useNavigate()

  const [name, setName] = useState("")
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [confirmPassword, setConfirmPassword] = useState("")
  const [error, setError] = useState("")
  const [submitting, setSubmitting] = useState(false)

  const handleSubmit = async (e) => {
    e.preventDefault()
    setError("")

    if (!name || !email || !password) {
      setError("Please fill out all fields")
      return
    }

    if (password.length < 6) {
      setError("Password must be at least 6 characters")
      return
    }

    if (password !== confirmPassword) {
      setError("Passwords do not match")
      return
    }

    setSubmitting(true)
    try {
      await register(name, email, password)
      navigate("/dashboard", { replace: true })
    } catch (err) {
      const msg = err.response?.data?.error?.message || "Failed to create account. Please try again."
      setError(msg)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="relative min-h-screen bg-white dark:bg-[#1c1c1c] text-neutral-900 dark:text-white flex flex-col justify-center items-center px-4 selection:bg-[#3ecf8e] selection:text-[#171717] transition-colors">
      {/* Top right theme switcher */}
      <div className="absolute top-5 right-5">
        <ThemeToggle showLabel={true} />
      </div>

      <div className="w-full max-w-md my-8">
        {/* Brand Header */}
        <div className="text-center mb-8">
          <div className="inline-flex items-center justify-center w-12 h-12 rounded-[6px] bg-[#3ecf8e]/10 border border-[#3ecf8e]/30 text-[#24b47e] dark:text-[#3ecf8e] mb-4 shadow-xs">
            <svg className="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
            </svg>
          </div>
          <h1 className="text-2xl font-bold tracking-tight text-neutral-900 dark:text-white">Create your account</h1>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">Start mapping, georeferencing, and collaborating on projects</p>
        </div>

        {/* Card */}
        <div className="bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] rounded-[12px] p-6 sm:p-8 shadow-xl">
          {error && (
            <div className="mb-6 p-3 rounded-[6px] bg-red-500/10 border border-red-500/20 text-red-500 dark:text-red-400 text-sm flex items-center gap-2">
              <svg className="w-4 h-4 shrink-0" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
              </svg>
              <span>{error}</span>
            </div>
          )}

          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="block text-xs font-semibold text-neutral-700 dark:text-neutral-300 uppercase tracking-wider mb-2">
                Full Name
              </label>
              <input
                type="text"
                autoComplete="name"
                required
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Jane Doe"
                className="w-full px-3.5 py-2.5 rounded-[6px] bg-neutral-50 dark:bg-[#171717] border border-neutral-300 dark:border-[#2e2e2e] text-neutral-900 dark:text-white placeholder-neutral-400 dark:placeholder-neutral-600 text-sm focus:outline-none focus:border-[#3ecf8e] transition-colors"
              />
            </div>

            <div>
              <label className="block text-xs font-semibold text-neutral-700 dark:text-neutral-300 uppercase tracking-wider mb-2">
                Email Address
              </label>
              <input
                type="email"
                autoComplete="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="name@organization.com"
                className="w-full px-3.5 py-2.5 rounded-[6px] bg-neutral-50 dark:bg-[#171717] border border-neutral-300 dark:border-[#2e2e2e] text-neutral-900 dark:text-white placeholder-neutral-400 dark:placeholder-neutral-600 text-sm focus:outline-none focus:border-[#3ecf8e] transition-colors"
              />
            </div>

            <div>
              <label className="block text-xs font-semibold text-neutral-700 dark:text-neutral-300 uppercase tracking-wider mb-2">
                Password
              </label>
              <input
                type="password"
                autoComplete="new-password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="At least 6 characters"
                className="w-full px-3.5 py-2.5 rounded-[6px] bg-neutral-50 dark:bg-[#171717] border border-neutral-300 dark:border-[#2e2e2e] text-neutral-900 dark:text-white placeholder-neutral-400 dark:placeholder-neutral-600 text-sm focus:outline-none focus:border-[#3ecf8e] transition-colors"
              />
            </div>

            <div>
              <label className="block text-xs font-semibold text-neutral-700 dark:text-neutral-300 uppercase tracking-wider mb-2">
                Confirm Password
              </label>
              <input
                type="password"
                autoComplete="new-password"
                required
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                placeholder="Repeat password"
                className="w-full px-3.5 py-2.5 rounded-[6px] bg-neutral-50 dark:bg-[#171717] border border-neutral-300 dark:border-[#2e2e2e] text-neutral-900 dark:text-white placeholder-neutral-400 dark:placeholder-neutral-600 text-sm focus:outline-none focus:border-[#3ecf8e] transition-colors"
              />
            </div>

            <button
              type="submit"
              disabled={submitting}
              className="w-full mt-2 py-2.5 px-4 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-medium text-sm transition-colors shadow-xs disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2 cursor-pointer"
            >
              {submitting ? (
                <>
                  <svg className="animate-spin -ml-1 mr-2 h-4 w-4 text-[#171717]" fill="none" viewBox="0 0 24 24">
                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
                  </svg>
                  Creating Account...
                </>
              ) : (
                "Create Account"
              )}
            </button>
          </form>

          <div className="mt-6 text-center text-xs text-neutral-500 dark:text-neutral-400">
            Already have an account?{" "}
            <Link to="/login" className="text-[#24b47e] dark:text-[#3ecf8e] hover:underline font-medium">
              Sign in
            </Link>
          </div>
        </div>
      </div>
    </div>
  )
}
