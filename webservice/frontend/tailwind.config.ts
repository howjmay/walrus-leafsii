import type { Config } from 'tailwindcss';

export default {
  darkMode: ['class'],
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        bg: { 
          base: '#0B0F1A', 
          card: '#121826', 
          card2: '#171C2F', 
          input: '#111827',
          gradientFrom: '#0B0F1A',
          gradientTo: '#121528'
        },
        brand: { 
          primary: '#8B5CF6', 
          soft: '#A78BFA',
          glow: 'rgba(139,92,246,0.25)'
        },
        text: { 
          primary: '#E5E7EB', 
          secondary: '#A3A7B3', 
          muted: '#8B90A0',
          onBrand: '#0B0F1A'
        },
        border: { 
          subtle: '#1F2937', 
          strong: '#273244', 
          focus: '#8B5CF6' 
        },
        success: '#22C55E', 
        warn: '#F59E0B', 
        danger: '#EF4444', 
        info: '#60A5FA',
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', 'sans-serif'],
      },
      fontSize: {
        'xs': ['12px', '16px'],
        'sm': ['14px', '20px'],
        'md': ['16px', '24px'],
        'lg': ['18px', '28px'],
        'xl': ['20px', '32px'],
        'display': ['32px', '40px'],
      },
      spacing: {
        '0': '0',
        '1': '4px',
        '2': '8px',
        '3': '12px',
        '4': '16px',
        '5': '20px',
        '6': '24px',
        '7': '28px',
        '8': '32px',
        '10': '40px',
        '12': '48px',
      },
      boxShadow: {
        'card': '0 8px 30px rgba(0,0,0,.35), 0 0 0 1px #1F2937 inset',
        'glow': '0 0 0 1px rgba(139,92,246,.35), 0 10px 40px rgba(139,92,246,.15)',
      },
      borderRadius: {
        'xl': '16px',
        '2xl': '20px',
      },
      backdropBlur: {
        'md': '12px',
      }
    },
  },
  plugins: [],
} satisfies Config;