import * as React from "react"
import { cn } from "@/lib/utils"

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'ghost'
  size?: 'sm' | 'md' | 'lg'
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = 'primary', size = 'md', ...props }, ref) => {
    const baseClasses = "inline-flex items-center justify-center font-semibold transition-all focus:outline-none focus:ring-2 focus:ring-border-focus disabled:opacity-55 disabled:cursor-not-allowed"
    
    const variants = {
      primary: "bg-brand-primary text-text-onBrand shadow-glow hover:opacity-95",
      secondary: "bg-bg-card border border-border-subtle text-text-primary hover:bg-bg-card2",
      ghost: "text-text-primary hover:bg-bg-card2"
    }
    
    const sizes = {
      sm: "h-8 px-3 text-sm rounded-lg",
      md: "h-10 px-4 text-md rounded-xl", 
      lg: "h-12 px-6 text-lg rounded-xl"
    }

    return (
      <button
        className={cn(
          baseClasses,
          variants[variant],
          sizes[size],
          className
        )}
        ref={ref}
        {...props}
      />
    )
  }
)
Button.displayName = "Button"