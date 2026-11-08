import React, { useState, useEffect } from "react"
import { Link, useNavigate } from "react-router-dom"
import { useAuth } from "../context/AuthContext"
import ThemeToggle from "../components/ThemeToggle"
import CodeSlots from "../components/CodeSlots"

export default function RegisterPage() {
  const { sendOtp, registerWithOtp } = useAuth()
  const navigate = useNavigate()

  const [step, setStep] = useState("details") // "details" | "otp"
  const [name, setName] = useState("")
  const [email, setEmail] = useState("")
  const [code, setCode] = useState("")
  const [otpStatus, setOtpStatus] = useState("idle") // "idle" | "error" | "success"
  const [error, setError] = useState("")
  const [info, setInfo] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const [cooldown, setCooldown] = useState(0)

  // Cooldown countdown timer
  useEffect(() => {
    if (cooldown <= 0) return
    const interval = setInterval(() => {
      setCooldown((prev) => (prev > 0 ? prev - 1 : 0))
    }, 1000)
    return () => clearInterval(interval)
  }, [cooldown])

  const handleSendCode = async (e) => {
    if (e) e.preventDefault()
    setError("")
    setInfo("")

    const cleanName = name.trim()
    const cleanEmail = email.trim().toLowerCase()

    if (!cleanName || !cleanEmail) {
      setError("Please provide your name and email address")
      return
    }

    setSubmitting(true)
    try {
      const resp = await sendOtp(cleanEmail, "register", cleanName)
      setStep("otp")
      setCode("")
      setOtpStatus("idle")
      setCooldown(resp?.resend_in_seconds || 60)
      setInfo(`A 6-digit verification code has been sent to ${cleanEmail}`)
    } catch (err) {
      const msg = err.response?.data?.error?.message || "Failed to send verification code. Please try again."
      setError(msg)
    } finally {
      setSubmitting(false)
    }
  }

  const handleVerifyOtp = async (inputCode) => {
    const finalCode = typeof inputCode === "string" ? inputCode : code
    setError("")

    if (finalCode.length !== 6) {
      setError("Please enter the complete 6-digit code")
      setOtpStatus("error")
      return
    }

    setSubmitting(true)
    try {
      await registerWithOtp(name.trim(), email.trim().toLowerCase(), finalCode)
      setOtpStatus("success")
      setTimeout(() => {
        navigate("/dashboard", { replace: true })
      }, 700)
    } catch (err) {
      const msg = err.response?.data?.error?.message || "Invalid or expired code. Please try again."
      setError(msg)
      setOtpStatus("error")
    } finally {
      setSubmitting(false)
    }
  }

  const handleBackToDetails = () => {
    setStep("details")
    setCode("")
    setOtpStatus("idle")
    setError("")
    setInfo("")
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
          <h1 className="text-2xl font-bold tracking-tight text-neutral-900 dark:text-white">
            {step === "details" ? "Create your account" : "Verify Your Email"}
          </h1>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
            {step === "details"
              ? "Passwordless registration with simple email verification"
              : `Enter the code sent to ${email}`}
          </p>
        </div>

        {/* Card */}
        <div className="bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] rounded-[12px] p-6 sm:p-8 shadow-xl">
          {error && (
            <div className="mb-6 p-3 rounded-[6px] bg-red-500/10 border border-red-500/20 text-red-500 dark:text-red-400 text-sm flex items-center justify-between gap-2">
              <div className="flex items-center gap-2">
                <svg className="w-4 h-4 shrink-0" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
                </svg>
                <span>{error}</span>
              </div>
              {error.toLowerCase().includes("already exists") && (
                <Link to="/login" className="text-[#24b47e] dark:text-[#3ecf8e] underline shrink-0 font-medium">
                  Sign In
                </Link>
              )}
            </div>
          )}

          {info && !error && (
            <div className="mb-6 p-3 rounded-[6px] bg-[#3ecf8e]/10 border border-[#3ecf8e]/30 text-[#24b47e] dark:text-[#3ecf8e] text-sm flex items-center gap-2">
              <svg className="w-4 h-4 shrink-0" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
              </svg>
              <span>{info}</span>
            </div>
          )}

          {step === "details" ? (
            <form onSubmit={handleSendCode} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-neutral-700 dark:text-neutral-300 uppercase tracking-wider mb-2">
                  Full Name
                </label>
                <input
                  type="text"
                  autoComplete="name"
                  required
                  autoFocus
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
                    Sending verification code...
                  </>
                ) : (
                  "Send Verification Code"
                )}
              </button>
            </form>
          ) : (
            <form onSubmit={handleVerifyOtp} className="space-y-5">
              <div className="flex items-center justify-between text-xs text-neutral-500 dark:text-neutral-400 pb-1">
                <span className="truncate max-w-[240px]">Account: <span className="font-semibold text-neutral-800 dark:text-neutral-200">{name} ({email})</span></span>
                <button
                  type="button"
                  onClick={handleBackToDetails}
                  className="text-[#24b47e] dark:text-[#3ecf8e] hover:underline font-medium cursor-pointer"
                >
                  Edit
                </button>
              </div>

              <div className="flex flex-col items-center">
                <label className="block text-xs font-semibold text-neutral-700 dark:text-neutral-300 uppercase tracking-wider mb-3 text-center">
                  6-Digit Verification Code
                </label>
                <div className="flex justify-center w-full my-1">
                  <CodeSlots
                    length={6}
                    value={code}
                    status={otpStatus}
                    disabled={submitting}
                    autoFocus={true}
                    onChange={(newCode) => {
                      setCode(newCode)
                      if (otpStatus !== "idle") setOtpStatus("idle")
                      if (error) setError("")
                    }}
                    onComplete={(completedCode) => {
                      handleVerifyOtp(completedCode)
                    }}
                    accentColor="#3ecf8e"
                    inkColor="#3ecf8e"
                    slotColor="#1c1c1c"
                    digitColor="#141414"
                    dangerColor="#ef4444"
                    slotSize={46}
                    gap={8}
                    radius={10}
                    bounce={0.2}
                    settle={0.3}
                    rise={8}
                    cascade={20}
                  />
                </div>
              </div>

              <button
                type="submit"
                disabled={submitting || code.length !== 6 || otpStatus === "success"}
                className="w-full py-2.5 px-4 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-medium text-sm transition-colors shadow-xs disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2 cursor-pointer"
              >
                {submitting ? (
                  <>
                    <svg className="animate-spin -ml-1 mr-2 h-4 w-4 text-[#171717]" fill="none" viewBox="0 0 24 24">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
                    </svg>
                    Creating account...
                  </>
                ) : (
                  "Verify & Create Account"
                )}
              </button>

              <div className="pt-2 flex items-center justify-between text-xs border-t border-neutral-100 dark:border-[#2e2e2e]">
                <span className="text-neutral-500 dark:text-neutral-400">Didn't receive code?</span>
                {cooldown > 0 ? (
                  <span className="text-neutral-400 dark:text-neutral-500 font-mono">
                    Resend in {cooldown}s
                  </span>
                ) : (
                  <button
                    type="button"
                    disabled={submitting}
                    onClick={() => handleSendCode()}
                    className="text-[#24b47e] dark:text-[#3ecf8e] hover:underline font-medium cursor-pointer disabled:opacity-50"
                  >
                    Resend Code
                  </button>
                )}
              </div>
            </form>
          )}

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
