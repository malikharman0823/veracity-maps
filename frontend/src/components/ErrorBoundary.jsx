import React from "react"
import { AlertCircle, RotateCcw } from "lucide-react"
import { Button } from "@/components/ui/button"

export default class ErrorBoundary extends React.Component {
  constructor(props) {
    super(props)
    this.state = { hasError: false, error: null }
  }

  static getDerivedStateFromError(error) {
    return { hasError: true, error }
  }

  componentDidCatch(error, errorInfo) {
    console.error("ErrorBoundary caught an error:", error, errorInfo)
  }

  handleReset = () => {
    this.setState({ hasError: false, error: null })
    if (this.props.onReset) {
      this.props.onReset()
    } else {
      window.location.reload()
    }
  }

  render() {
    if (this.state.hasError) {
      if (this.props.fallback) {
        return this.props.fallback
      }
      return (
        <div className="flex flex-col items-center justify-center min-h-[300px] h-full w-full p-8 bg-[#fafafc] border border-[#e0e0e0] rounded-2xl text-center">
          <div className="size-12 rounded-xl bg-[#ff3b30]/10 border border-[#ff3b30]/20 flex items-center justify-center text-[#ff3b30] mb-4">
            <AlertCircle className="size-6" />
          </div>
          <h3 className="text-sm font-semibold text-[#1d1d1f] mb-1">
            {this.props.title || "Something went wrong in this component"}
          </h3>
          <p className="text-xs text-[#7a7a7a] max-w-md mb-5 font-mono">
            {this.state.error?.message || "An unexpected error occurred during rendering."}
          </p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={this.handleReset}
            className="rounded-full border-[#e0e0e0] text-xs gap-1.5"
          >
            <RotateCcw className="size-3.5" />
            <span>Reload View</span>
          </Button>
        </div>
      )
    }

    return this.props.children
  }
}
