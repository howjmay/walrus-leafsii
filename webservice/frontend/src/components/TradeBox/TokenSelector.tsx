import { useState, useRef, useEffect } from 'react'
import { createPortal } from 'react-dom'
import { ChevronDown } from 'lucide-react'

interface Token {
  symbol: string
  name: string
  color: string
}

const tokens: Token[] = [
  { symbol: 'fToken', name: 'Stable fToken (β=0)', color: 'bg-blue-500' },
  { symbol: 'xToken', name: 'Leveraged xToken', color: 'bg-purple-500' },
  { symbol: 'Sui', name: 'Sui Token', color: 'bg-green-500' },
  { symbol: 'SUI', name: 'Sui', color: 'bg-indigo-500' },
]

interface TokenSelectorProps {
  selectedToken: string
  onTokenChange: (token: string) => void
  excludeToken?: string
}

export function TokenSelector({ selectedToken, onTokenChange, excludeToken }: TokenSelectorProps) {
  const [isOpen, setIsOpen] = useState(false)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const [dropdownPosition, setDropdownPosition] = useState({ top: 0, left: 0, width: 0 })
  
  const availableTokens = tokens.filter(token => token.symbol !== excludeToken)
  const selected = tokens.find(token => token.symbol === selectedToken)

  // Calculate dropdown position when opened
  useEffect(() => {
    if (isOpen && buttonRef.current) {
      const rect = buttonRef.current.getBoundingClientRect()
      setDropdownPosition({
        top: rect.bottom + 4, // 4px gap (mt-1 equivalent)
        left: rect.right - 192, // Align right edge (min-w-48 = 192px)
        width: Math.max(rect.width, 192) // Minimum width of 192px
      })
    }
  }, [isOpen])

  // Close dropdown on escape key
  useEffect(() => {
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setIsOpen(false)
      }
    }

    if (isOpen) {
      document.addEventListener('keydown', handleEscape)
      return () => document.removeEventListener('keydown', handleEscape)
    }
  }, [isOpen])

  const dropdownContent = isOpen && (
    <div 
      className="fixed min-w-48 bg-bg-card border border-border-subtle rounded-xl shadow-card z-[9999]"
      style={{
        top: dropdownPosition.top,
        left: dropdownPosition.left,
        width: dropdownPosition.width
      }}
    >
      {availableTokens.map((token) => (
        <button
          key={token.symbol}
          onClick={() => {
            onTokenChange(token.symbol)
            setIsOpen(false)
          }}
          className={`w-full px-4 py-3 text-left hover:bg-bg-card2 transition-colors first:rounded-t-xl last:rounded-b-xl ${
            token.symbol === selectedToken ? 'bg-bg-card2' : ''
          }`}
        >
          <div className="flex items-center gap-3">
            <div className={`w-6 h-6 rounded-full ${token.color} flex-shrink-0`} />
            <div>
              <div className="text-text-primary font-medium">{token.symbol}</div>
              <div className="text-text-muted text-sm">{token.name}</div>
            </div>
          </div>
        </button>
      ))}
    </div>
  )

  return (
    <div className="relative">
      <button
        ref={buttonRef}
        onClick={() => setIsOpen(!isOpen)}
        className="flex items-center gap-2 px-3 py-2 bg-bg-card2 border border-border-subtle rounded-lg hover:bg-bg-input transition-colors"
      >
        <div className={`w-5 h-5 rounded-full ${selected?.color}`} />
        <span className="text-text-primary font-medium">{selected?.symbol}</span>
        <ChevronDown className={`w-4 h-4 transition-transform ${isOpen ? 'rotate-180' : ''}`} />
      </button>

      {/* Portal the dropdown to document.body to avoid z-index issues */}
      {dropdownContent && createPortal(dropdownContent, document.body)}

      {/* Backdrop overlay */}
      {isOpen && createPortal(
        <div 
          className="fixed inset-0 z-[9998]" 
          onClick={() => setIsOpen(false)}
        />,
        document.body
      )}
    </div>
  )
}